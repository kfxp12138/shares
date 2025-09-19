package analy

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"shares/internal/api"
	"shares/internal/core"

	"github.com/xuri/excelize/v2"
	"github.com/xxjwxc/public/mysqldb"
	"github.com/xxjwxc/public/tools"
)

type limitupPoolExportReq struct {
	Date string `form:"date" json:"date"`
}

type stockLimitupMetrics struct {
	Code         string
	Name         string
	HyName       string
	Percent      float64
	ClosePrice   float64
	Consecutive  int
	LimitUps5d   int
	LimitUps3d   int
	PctChange5d  float64
	PctChange10d float64
}

type conceptLimitupSummary struct {
	Name         string
	HyCode       string
	Stocks       []*stockLimitupMetrics
	MaxConsec    int
	MaxFiveDay   int
	MaxThreeDay  int
	PctChange5d  float64
	PctChange10d float64
}

// LimitupPoolExport 接收股票池文件，匹配当日涨停股概念并导出 Excel
// 路由：POST /analy.limitup_pool_export  (multipart/form-data: file=..., 可选 date=YYYY-MM-DD)
func LimitupPoolExport(c *api.Context) {
	ctx := c.GetGinCtx()

	var req limitupPoolExportReq
	_ = ctx.ShouldBind(&req)
	if req.Date == "" {
		req.Date = ctx.PostForm("date")
	}
	if req.Date == "" {
		req.Date = ctx.Query("date")
	}

	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, map[string]string{"err": "file required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, map[string]string{"err": fmt.Sprintf("open upload failed: %v", err)})
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, map[string]string{"err": fmt.Sprintf("read upload failed: %v", err)})
		return
	}

	codes, err := parseCodesFromUpload(content, fileHeader.Filename)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, map[string]string{"err": err.Error()})
		return
	}
	if len(codes) == 0 {
		ctx.JSON(http.StatusBadRequest, map[string]string{"err": "no stock codes found in upload"})
		return
	}
	codes = normalizeCodes(codes)

	orm := core.Dao.GetDBr()

	// 解析目标交易日
	targetDay0, dayStr, err := resolveTargetDay(orm, strings.TrimSpace(req.Date))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, map[string]string{"err": err.Error()})
		return
	}

	// 股票池概念集合（canonical -> summary）
	poolConcepts := buildPoolConceptSet(codes)
	if len(poolConcepts) == 0 {
		ctx.JSON(http.StatusBadRequest, map[string]string{"err": "no concepts resolved from stock pool"})
		return
	}
	fillConceptHyCodes(poolConcepts)

	// 获取目标日全部股票的日线数据
	todays, err := fetchDailyRowsForDay(orm, targetDay0)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
		return
	}
	if len(todays) == 0 {
		ctx.JSON(http.StatusBadRequest, map[string]string{"err": "no daily rows for target date"})
		return
	}

	// 筛选涨停股
	stockMetrics := map[string]*stockLimitupMetrics{}
	var limitupCodes []string
	for _, row := range todays {
		if !isLimitUp(row.Percent, row.Code, row.HyName) {
			continue
		}
		metrics := &stockLimitupMetrics{
			Code:       strings.ToLower(row.Code),
			Name:       row.Name,
			HyName:     row.HyName,
			Percent:    round2(row.Percent),
			ClosePrice: row.ClosePrice,
		}
		stockMetrics[metrics.Code] = metrics
		limitupCodes = append(limitupCodes, metrics.Code)
	}
	if len(stockMetrics) == 0 {
		ctx.JSON(http.StatusBadRequest, map[string]string{"err": "no limit-up stocks found for target date"})
		return
	}

	// 限于股票池概念筛选涨停股
	conceptsByLimitup := conceptsByCodes(limitupCodes)
	conceptSummaries := map[string]*conceptLimitupSummary{}
	for canonical := range poolConcepts {
		conceptSummaries[canonical] = &conceptLimitupSummary{Name: canonical, HyCode: poolConcepts[canonical].HyCode}
	}

	for code, metrics := range stockMetrics {
		lst := conceptsByLimitup[code]
		if len(lst) == 0 {
			continue
		}
		matched := false
		for _, raw := range lst {
			canonical := canonicalizeConcept(raw)
			if summary, ok := conceptSummaries[canonical]; ok {
				summary.Stocks = append(summary.Stocks, metrics)
				matched = true
			}
		}
		if !matched {
			delete(stockMetrics, code)
		}
	}

	// 移除没有匹配涨停股的概念
	for name, summary := range conceptSummaries {
		if len(summary.Stocks) == 0 {
			delete(conceptSummaries, name)
		}
	}
	if len(conceptSummaries) == 0 {
		ctx.JSON(http.StatusBadRequest, map[string]string{"err": "limit-up stocks do not intersect with pool concepts"})
		return
	}

	// 历史数据计算个股指标
	if err := enrichStockMetricsWithHistory(orm, stockMetrics, targetDay0); err != nil {
		ctx.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
		return
	}

	// 计算概念层级指标
	if err := enrichConceptSummaries(orm, poolConcepts, conceptSummaries, targetDay0); err != nil {
		ctx.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
		return
	}

	// 构建排序后的概念数组
	var ordered []*conceptLimitupSummary
	for _, summary := range conceptSummaries {
		summary.aggregateFromStocks()
		ordered = append(ordered, summary)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].MaxConsec != ordered[j].MaxConsec {
			return ordered[i].MaxConsec > ordered[j].MaxConsec
		}
		if ordered[i].MaxFiveDay != ordered[j].MaxFiveDay {
			return ordered[i].MaxFiveDay > ordered[j].MaxFiveDay
		}
		if ordered[i].MaxThreeDay != ordered[j].MaxThreeDay {
			return ordered[i].MaxThreeDay > ordered[j].MaxThreeDay
		}
		if !floatEqual(ordered[i].PctChange5d, ordered[j].PctChange5d) {
			return ordered[i].PctChange5d > ordered[j].PctChange5d
		}
		if !floatEqual(ordered[i].PctChange10d, ordered[j].PctChange10d) {
			return ordered[i].PctChange10d > ordered[j].PctChange10d
		}
		return ordered[i].Name < ordered[j].Name
	})

	poolFilename := strings.TrimSpace(fileHeader.Filename)
	if poolFilename != "" {
		poolFilename = filepath.Base(poolFilename)
	}
	if poolFilename == "" {
		poolFilename = "upload.txt"
	}
	if err := persistLimitupReport(strings.TrimSpace(dayStr), poolFilename, ordered, stockMetrics); err != nil {
		ctx.JSON(http.StatusInternalServerError, map[string]string{"err": fmt.Sprintf("save report failed: %v", err)})
		return
	}

	workbook, err := buildLimitupExcel(ordered)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
		return
	}
	defer workbook.Close()

	buf, err := workbook.WriteToBuffer()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, map[string]string{"err": fmt.Sprintf("write excel failed: %v", err)})
		return
	}
	filename := fmt.Sprintf("limitup_concepts_%s.xlsx", strings.ReplaceAll(dayStr, "-", ""))
	disposition := fmt.Sprintf(`attachment; filename="%s"`, filename)
	ctx.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	ctx.Header("Content-Disposition", disposition)
	ctx.Header("Content-Length", fmt.Sprintf("%d", buf.Len()))
	ctx.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

type dailyRow struct {
	Code       string
	Percent    float64
	ClosePrice float64
	Day0       int64
	HyName     string
	Name       string
}

type poolConceptInfo struct {
	HyCode string
}

func parseCodesFromUpload(content []byte, filename string) ([]string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".xlsx" || ext == ".xlsm" || ext == ".xltx" || ext == ".xltm" {
		return parseCodesFromExcel(content)
	}
	if ext == ".csv" {
		return parseCodesFromCSV(content)
	}
	// 退化为纯文本解析
	return splitCodesFlexible(string(content)), nil
}

func parseCodesFromExcel(content []byte) ([]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("parse excel failed: %w", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("excel has no sheets")
	}
	seen := map[string]struct{}{}
	var out []string
	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		for _, row := range rows {
			for _, cell := range row {
				for _, code := range splitCodesFlexible(cell) {
					code = strings.TrimSpace(code)
					if code == "" {
						continue
					}
					code = normalizeCodeMarket(strings.ToLower(code))
					if code == "" {
						continue
					}
					if _, ok := seen[code]; ok {
						continue
					}
					seen[code] = struct{}{}
					out = append(out, code)
				}
			}
		}
	}
	return out, nil
}

func parseCodesFromCSV(content []byte) ([]string, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	var out []string
	seen := map[string]struct{}{}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse csv failed: %w", err)
		}
		for _, cell := range record {
			for _, code := range splitCodesFlexible(cell) {
				code = strings.TrimSpace(code)
				if code == "" {
					continue
				}
				code = normalizeCodeMarket(strings.ToLower(code))
				if code == "" {
					continue
				}
				if _, ok := seen[code]; ok {
					continue
				}
				seen[code] = struct{}{}
				out = append(out, code)
			}
		}
	}
	return out, nil
}

func resolveTargetDay(orm *mysqldb.MySqlDB, dateStr string) (int64, string, error) {
	var targetDay0 int64
	if strings.TrimSpace(dateStr) != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return 0, "", fmt.Errorf("invalid date format, expect YYYY-MM-DD")
		}
		targetDay0 = tools.GetUtcDay0(parsed)
		type row struct {
			Day0    int64
			Day0Str string
		}
		var r row
		orm.DB.Raw("SELECT day0, day0_str FROM shares_daily_tbl WHERE day0 <= ? ORDER BY day0 DESC LIMIT 1", targetDay0).Scan(&r)
		if r.Day0 == 0 {
			return 0, "", fmt.Errorf("no daily data on or before %s", dateStr)
		}
		return r.Day0, strings.TrimSpace(r.Day0Str), nil
	}

	type row struct {
		Day0    int64
		Day0Str string
	}
	var r row
	orm.DB.Raw("SELECT day0, day0_str FROM shares_daily_tbl ORDER BY day0 DESC LIMIT 1").Scan(&r)
	if r.Day0 == 0 {
		return 0, "", fmt.Errorf("shares_daily_tbl empty")
	}
	return r.Day0, strings.TrimSpace(r.Day0Str), nil
}

func buildPoolConceptSet(codes []string) map[string]*poolConceptInfo {
	concepts := conceptsByCodes(codes)
	out := map[string]*poolConceptInfo{}
	for _, lst := range concepts {
		for _, raw := range lst {
			canonical := canonicalizeConcept(raw)
			if canonical == "" {
				continue
			}
			if _, ok := out[canonical]; !ok {
				out[canonical] = &poolConceptInfo{}
			}
		}
	}
	return out
}

func fillConceptHyCodes(poolConcepts map[string]*poolConceptInfo) {
	if len(poolConcepts) == 0 {
		return
	}
	var names []string
	for name := range poolConcepts {
		names = append(names, name)
	}
	type row struct {
		Name   string
		HyCode string
	}
	var rows []row
	core.Dao.GetDBr().Raw("SELECT name, hy_code FROM concept_master_tbl WHERE name IN (?)", names).Scan(&rows)
	for _, r := range rows {
		canonical := canonicalizeConcept(r.Name)
		if info, ok := poolConcepts[canonical]; ok {
			info.HyCode = strings.TrimSpace(r.HyCode)
		}
	}
}

func fetchDailyRowsForDay(orm *mysqldb.MySqlDB, day0 int64) ([]dailyRow, error) {
	var rows []dailyRow
	err := orm.DB.Raw(`
        SELECT d.code, d.percent, d.close_price, d.day0, IFNULL(i.hy_name, '') AS hy_name, IFNULL(i.name, '') AS name
        FROM shares_daily_tbl d
        LEFT JOIN shares_info_tbl i ON i.code = d.code
        WHERE d.day0 = ?
    `, day0).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Code = strings.ToLower(strings.TrimSpace(rows[i].Code))
		rows[i].HyName = strings.TrimSpace(rows[i].HyName)
		rows[i].Name = strings.TrimSpace(rows[i].Name)
	}
	return rows, nil
}

func enrichStockMetricsWithHistory(orm *mysqldb.MySqlDB, metrics map[string]*stockLimitupMetrics, targetDay0 int64) error {
	if len(metrics) == 0 {
		return nil
	}
	var codes []string
	for code := range metrics {
		codes = append(codes, code)
	}
	since := targetDay0 - int64(30*24*60*60)
	type histRow struct {
		Code       string
		Day0       int64
		Percent    float64
		ClosePrice float64
	}
	var hist []histRow
	if err := orm.DB.Raw(`
        SELECT code, day0, percent, close_price
        FROM shares_daily_tbl
        WHERE code IN (?) AND day0 <= ? AND day0 >= ?
        ORDER BY code, day0 DESC
    `, codes, targetDay0, since).Scan(&hist).Error; err != nil {
		return err
	}
	grouped := map[string][]histRow{}
	for _, row := range hist {
		code := strings.ToLower(strings.TrimSpace(row.Code))
		grouped[code] = append(grouped[code], row)
	}
	for code, rows := range grouped {
		metricsRow := metrics[code]
		if metricsRow == nil {
			continue
		}
		if len(rows) == 0 {
			continue
		}
		// rows 已按 day0 desc 排序，rows[0] 应为 targetDay0
		idx := -1
		for i, r := range rows {
			if r.Day0 == targetDay0 {
				idx = i
				break
			}
		}
		if idx == -1 {
			continue
		}
		// 连续涨停天数
		streak := 0
		for i := idx; i < len(rows); i++ {
			if isLimitUp(rows[i].Percent, code, metricsRow.HyName) {
				streak++
			} else {
				break
			}
		}
		metricsRow.Consecutive = streak
		// 近5日、3日涨停次数
		for i := 0; i < len(rows) && i < 5; i++ {
			if isLimitUp(rows[i].Percent, code, metricsRow.HyName) {
				metricsRow.LimitUps5d++
				if i < 3 {
					metricsRow.LimitUps3d++
				}
			}
		}
		// 5日、10日涨幅
		if len(rows) > 4 && rows[4].ClosePrice > 0 && rows[0].ClosePrice > 0 {
			metricsRow.PctChange5d = round2((rows[0].ClosePrice/rows[4].ClosePrice - 1) * 100)
		}
		if len(rows) > 9 && rows[9].ClosePrice > 0 && rows[0].ClosePrice > 0 {
			metricsRow.PctChange10d = round2((rows[0].ClosePrice/rows[9].ClosePrice - 1) * 100)
		}
	}
	return nil
}

func enrichConceptSummaries(orm *mysqldb.MySqlDB, poolConcepts map[string]*poolConceptInfo, summaries map[string]*conceptLimitupSummary, targetDay0 int64) error {
	hyCodes := map[string]struct{}{}
	for name, info := range poolConcepts {
		summary := summaries[name]
		if summary == nil {
			continue
		}
		if strings.TrimSpace(info.HyCode) != "" {
			hyCodes[strings.TrimSpace(info.HyCode)] = struct{}{}
			summary.HyCode = strings.TrimSpace(info.HyCode)
		}
	}
	if len(hyCodes) == 0 {
		return nil
	}
	var hyList []string
	for hy := range hyCodes {
		hyList = append(hyList, hy)
	}
	since := targetDay0 - int64(30*24*60*60)
	type hyRow struct {
		HyCode     string
		Day0       int64
		ClosePrice float64
	}
	var rows []hyRow
	if err := orm.DB.Raw(`
        SELECT hy_code, day0, close_price
        FROM hy_daily_tbl
        WHERE hy_code IN (?) AND day0 <= ? AND day0 >= ?
        ORDER BY hy_code, day0 DESC
    `, hyList, targetDay0, since).Scan(&rows).Error; err != nil {
		return err
	}
	grouped := map[string][]hyRow{}
	for _, r := range rows {
		grouped[strings.TrimSpace(r.HyCode)] = append(grouped[strings.TrimSpace(r.HyCode)], r)
	}
	for name, summary := range summaries {
		if summary.HyCode == "" {
			continue
		}
		lst := grouped[summary.HyCode]
		if len(lst) == 0 {
			continue
		}
		if lst[0].Day0 != targetDay0 {
			continue
		}
		if len(lst) > 4 && lst[4].ClosePrice > 0 && lst[0].ClosePrice > 0 {
			summary.PctChange5d = round2((lst[0].ClosePrice/lst[4].ClosePrice - 1) * 100)
		}
		if len(lst) > 9 && lst[9].ClosePrice > 0 && lst[0].ClosePrice > 0 {
			summary.PctChange10d = round2((lst[0].ClosePrice/lst[9].ClosePrice - 1) * 100)
		}
		summaries[name] = summary
	}
	return nil
}

func (summary *conceptLimitupSummary) aggregateFromStocks() {
	if summary == nil {
		return
	}
	total5 := 0.0
	total10 := 0.0
	count5 := 0
	count10 := 0
	for _, s := range summary.Stocks {
		if s.Consecutive > summary.MaxConsec {
			summary.MaxConsec = s.Consecutive
		}
		if s.LimitUps5d > summary.MaxFiveDay {
			summary.MaxFiveDay = s.LimitUps5d
		}
		if s.LimitUps3d > summary.MaxThreeDay {
			summary.MaxThreeDay = s.LimitUps3d
		}
		if !floatEqual(s.PctChange5d, 0) {
			total5 += s.PctChange5d
			count5++
		}
		if !floatEqual(s.PctChange10d, 0) {
			total10 += s.PctChange10d
			count10++
		}
	}
	if floatEqual(summary.PctChange5d, 0) && count5 > 0 {
		summary.PctChange5d = round2(total5 / float64(count5))
	}
	if floatEqual(summary.PctChange10d, 0) && count10 > 0 {
		summary.PctChange10d = round2(total10 / float64(count10))
	}
}

func buildLimitupExcel(summaries []*conceptLimitupSummary) (*excelize.File, error) {
	wb := excelize.NewFile()
	summarySheet := "Summary"
	wb.SetSheetName("Sheet1", summarySheet)
	headers := []string{"Concept", "HyCode", "MatchedStocks", "MaxConsecutiveLimitUps", "Max5DayLimitUps", "Max3DayLimitUps", "PctChange5d", "PctChange10d"}
	for i, h := range headers {
		cell := excelColumn(i) + "1"
		wb.SetCellValue(summarySheet, cell, h)
	}
	for idx, summary := range summaries {
		row := idx + 2
		wb.SetCellValue(summarySheet, excelColumn(0)+fmt.Sprint(row), summary.Name)
		wb.SetCellValue(summarySheet, excelColumn(1)+fmt.Sprint(row), summary.HyCode)
		wb.SetCellValue(summarySheet, excelColumn(2)+fmt.Sprint(row), len(summary.Stocks))
		wb.SetCellValue(summarySheet, excelColumn(3)+fmt.Sprint(row), summary.MaxConsec)
		wb.SetCellValue(summarySheet, excelColumn(4)+fmt.Sprint(row), summary.MaxFiveDay)
		wb.SetCellValue(summarySheet, excelColumn(5)+fmt.Sprint(row), summary.MaxThreeDay)
		wb.SetCellValue(summarySheet, excelColumn(6)+fmt.Sprint(row), summary.PctChange5d)
		wb.SetCellValue(summarySheet, excelColumn(7)+fmt.Sprint(row), summary.PctChange10d)
	}

	_ = wb.SetColWidth(summarySheet, "A", excelColumn(len(headers)-1), 18)

	usedSheets := map[string]struct{}{summarySheet: {}}
	for idx, summary := range summaries {
		sheet := sanitizeSheetName(fmt.Sprintf("%d_%s", idx+1, summary.Name))
		for {
			if _, exists := usedSheets[sheet]; !exists {
				break
			}
			sheet = sheet + "_"
		}
		usedSheets[sheet] = struct{}{}
		wb.NewSheet(sheet)
		headers := []string{"Code", "Name", "Percent", "ConsecutiveLimitUps", "LimitUps5d", "LimitUps3d", "PctChange5d", "PctChange10d"}
		for i, h := range headers {
			wb.SetCellValue(sheet, excelColumn(i)+"1", h)
		}
		_ = wb.SetColWidth(sheet, "A", excelColumn(len(headers)-1), 16)
		for i, stock := range summary.Stocks {
			row := i + 2
			wb.SetCellValue(sheet, excelColumn(0)+fmt.Sprint(row), stock.Code)
			wb.SetCellValue(sheet, excelColumn(1)+fmt.Sprint(row), stock.Name)
			wb.SetCellValue(sheet, excelColumn(2)+fmt.Sprint(row), stock.Percent)
			wb.SetCellValue(sheet, excelColumn(3)+fmt.Sprint(row), stock.Consecutive)
			wb.SetCellValue(sheet, excelColumn(4)+fmt.Sprint(row), stock.LimitUps5d)
			wb.SetCellValue(sheet, excelColumn(5)+fmt.Sprint(row), stock.LimitUps3d)
			wb.SetCellValue(sheet, excelColumn(6)+fmt.Sprint(row), stock.PctChange5d)
			wb.SetCellValue(sheet, excelColumn(7)+fmt.Sprint(row), stock.PctChange10d)
		}
	}

	if idx, err := wb.GetSheetIndex(summarySheet); err == nil {
		wb.SetActiveSheet(idx)
	}
	return wb, nil
}

func excelColumn(idx int) string {
	col := ""
	i := idx + 1
	for i > 0 {
		i--
		col = string(rune('A'+i%26)) + col
		i /= 26
	}
	return col
}

func sanitizeSheetName(name string) string {
	sanitized := name
	for _, ch := range []string{"/", "\\", "*", "?", "[", "]", ":"} {
		sanitized = strings.ReplaceAll(sanitized, ch, " ")
	}
	sanitized = strings.TrimSpace(sanitized)
	if len([]rune(sanitized)) > 28 {
		sanitized = string([]rune(sanitized)[:28])
	}
	if sanitized == "" {
		sanitized = "Sheet"
	}
	return sanitized
}

func floatEqual(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-6
}

type limitupReportRun struct {
	ID           int64
	RunAt        time.Time
	TradeDay     time.Time
	PoolFilename string
	ConceptCount int
	StockCount   int
}

type limitupReportItem struct {
    ID            int64     `gorm:"column:id"`
    RunID         int64     `gorm:"column:run_id"`
    ConceptName   string    `gorm:"column:concept_name"`
    HyCode        string    `gorm:"column:hy_code"`
    StockCode     string    `gorm:"column:stock_code"`
    StockName     string    `gorm:"column:stock_name"`
    Consecutive   int       `gorm:"column:consecutive"`
    Limitups3d    int       `gorm:"column:limitups_3d"`
    Limitups5d    int       `gorm:"column:limitups_5d"`
    PctChange5d   float64   `gorm:"column:pct_change_5d"`
    PctChange10d  float64   `gorm:"column:pct_change_10d"`
    ConceptPct5d  float64   `gorm:"column:concept_pct_5d"`
    ConceptPct10d float64   `gorm:"column:concept_pct_10d"`
    CreatedAt     time.Time `gorm:"column:created_at"`
}

func persistLimitupReport(tradeDay, poolFilename string, summaries []*conceptLimitupSummary, metrics map[string]*stockLimitupMetrics) error {
	db := core.Dao.GetDBw()
	if db == nil {
		return fmt.Errorf("db unavailable")
	}
	if err := ensureLimitupTables(db); err != nil {
		return err
	}
	runAt := time.Now()
	tradeTime, err := time.Parse("2006-01-02", tradeDay)
	if err != nil {
		tradeTime = runAt
	}
	run := &limitupReportRun{
		RunAt:        runAt,
		TradeDay:     tradeTime,
		PoolFilename: poolFilename,
		ConceptCount: len(summaries),
		StockCount:   len(metrics),
	}
	if err := db.DB.Table("limitup_report_run").Create(run).Error; err != nil {
		return err
	}

	totalStocks := 0
	for _, summary := range summaries {
		totalStocks += len(summary.Stocks)
	}
	if totalStocks == 0 {
		return nil
	}

	items := make([]limitupReportItem, 0, totalStocks)
	for _, summary := range summaries {
		for _, stock := range summary.Stocks {
			items = append(items, limitupReportItem{
				RunID:         run.ID,
				ConceptName:   summary.Name,
				HyCode:        summary.HyCode,
				StockCode:     stock.Code,
				StockName:     stock.Name,
				Consecutive:   stock.Consecutive,
				Limitups3d:    stock.LimitUps3d,
				Limitups5d:    stock.LimitUps5d,
				PctChange5d:   stock.PctChange5d,
				PctChange10d:  stock.PctChange10d,
				ConceptPct5d:  summary.PctChange5d,
				ConceptPct10d: summary.PctChange10d,
				CreatedAt:     runAt,
			})
		}
	}
	if len(items) == 0 {
		return nil
	}
	return db.DB.Table("limitup_report_item").Create(&items).Error
}

func ensureLimitupTables(db *mysqldb.MySqlDB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS limitup_report_run (
            id BIGINT NOT NULL AUTO_INCREMENT,
            run_at DATETIME NOT NULL,
            trade_day DATE NOT NULL,
            pool_filename VARCHAR(255) DEFAULT '',
            concept_count INT NOT NULL DEFAULT 0,
            stock_count INT NOT NULL DEFAULT 0,
            PRIMARY KEY (id),
            KEY idx_trade_day (trade_day)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS limitup_report_item (
            id BIGINT NOT NULL AUTO_INCREMENT,
            run_id BIGINT NOT NULL,
            concept_name VARCHAR(128) NOT NULL,
            hy_code VARCHAR(16) DEFAULT '',
            stock_code VARCHAR(16) NOT NULL,
            stock_name VARCHAR(64) DEFAULT '',
            consecutive INT NOT NULL DEFAULT 0,
            limitups_3d INT NOT NULL DEFAULT 0,
            limitups_5d INT NOT NULL DEFAULT 0,
            pct_change_5d DOUBLE DEFAULT 0,
            pct_change_10d DOUBLE DEFAULT 0,
            concept_pct_5d DOUBLE DEFAULT 0,
            concept_pct_10d DOUBLE DEFAULT 0,
            created_at DATETIME NOT NULL,
            PRIMARY KEY (id),
            KEY idx_run_concept (run_id, concept_name),
            KEY idx_stock (stock_code),
            CONSTRAINT fk_limitup_item_run FOREIGN KEY (run_id) REFERENCES limitup_report_run(id) ON DELETE CASCADE
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
	}
	for _, stmt := range stmts {
		if err := db.DB.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
