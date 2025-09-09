package analy

import (
    "net/http"
    "sort"
    "strings"
    "sync"
    "time"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
    "shares/internal/service/event"

    "github.com/xxjwxc/public/tools"
)

// SameConceptLimitupCalendarReq 请求：同概念涨停日历
type SameConceptLimitupCalendarReq struct {
    Code        string `json:"code" form:"code"`
    Days        int    `json:"days" form:"days"`
    PerConcept  int    `json:"perConcept" form:"perConcept"`
    Concepts    []string `json:"concepts" form:"concepts"` // 选中的概念（可多值或逗号分隔字符串）
    // 统计口径：limitup(涨停) / bigrise(大涨，按最小涨幅判断)
    Mode        string  `json:"mode" form:"mode"`
    // 大涨最小阈值（单位 %），配合 Mode=bigrise 使用，默认 7
    MinPct      float64 `json:"minPct" form:"minPct"`
    // 大涨相对阈值（相对各自涨停幅度的比例 0~1，例如 0.7 表示 ≥ 涨停的 70%）；若 >0 优先生效
    MinRate     float64 `json:"minRate" form:"minRate"`
}

type CalendarItem struct {
    Date   string `json:"date"`
    Count  int    `json:"count"`
    // 主股票是否当日也达到口径（用于前端特别标记）
    MainUp bool   `json:"mainUp"`
}

type SameConceptLimitupCalendarResp struct {
    Code     string         `json:"code"`
    Concepts []string       `json:"concepts"`
    Codes    int            `json:"codes"`
    Items    []CalendarItem `json:"items"`
}

// SameConceptLimitupCalendar 返回最近 N 个交易日中，同概念成分股的涨停数量（按日汇总）
func SameConceptLimitupCalendar(c *api.Context) {
    req := SameConceptLimitupCalendarReq{ Days: 90, PerConcept: 60 }
    _ = c.GetGinCtx().ShouldBind(&req)
    if strings.TrimSpace(req.Code) == "" {
        c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err": "code required"})
        return
    }
    if req.Days <= 0 { req.Days = 90 }
    if req.PerConcept <= 0 { req.PerConcept = 60 }
    // normalize mode & minPct
    mode := strings.ToLower(strings.TrimSpace(req.Mode))
    if mode == "" { mode = "limitup" }
    minPct := req.MinPct
    if minPct <= 0 { minPct = 7 }
    minRate := req.MinRate

    code := strings.ToLower(strings.TrimSpace(req.Code))
    // 概念列表
    concepts := conceptsForCode(code)
    // 可选：按传入 concepts 进行过滤，减少计算量
    if len(req.Concepts) == 0 {
        // 支持 query/form 的逗号分隔
        raw := c.GetGinCtx().Query("concepts"); if raw == "" { raw = c.GetGinCtx().PostForm("concepts") }
        if strings.TrimSpace(raw) != "" { req.Concepts = splitConceptsFlexible(raw) }
    }
    if len(req.Concepts) > 0 {
        concepts = filterConceptsBySelection(concepts, req.Concepts)
    }
    if len(concepts) == 0 {
        c.GetGinCtx().JSON(http.StatusOK, &SameConceptLimitupCalendarResp{ Code: code, Concepts: []string{}, Codes: 0, Items: []CalendarItem{} })
        return
    }

    // 候选代码集合
    candSet := make(map[string]struct{})
    for _, name := range concepts {
        for _, co := range codesByConcept(name, req.PerConcept) {
            if co == "" || strings.EqualFold(co, code) { continue }
            candSet[co] = struct{}{}
        }
    }
    // 归集候选
    var candidates []string
    for k := range candSet { candidates = append(candidates, k) }

    // 基准交易日（使用主股票日K，近 N 日）
    baseDates := lastTradingDaysByCode(code, req.Days)
    if len(baseDates) == 0 {
        c.GetGinCtx().JSON(http.StatusOK, &SameConceptLimitupCalendarResp{ Code: code, Concepts: concepts, Codes: len(candidates), Items: []CalendarItem{} })
        return
    }
    // 初始化计数
    countMp := make(map[string]int, len(baseDates))
    dateSet := make(map[string]struct{}, len(baseDates))
    for _, d := range baseDates { countMp[d] = 0; dateSet[d] = struct{}{} }

    // 每只股票按 shares_daily_tbl 快速判断；若表中不存在则回退至网络日K
    // 预拉取 hy_name（用于 ST/20%/10% 阈值判断）
    hyMp := hyNameByCodes(candidates)

    // 主股票当日是否也达到口径（用于“同时大涨/涨停”特别标记）
    mainHy := hyNameByCodes([]string{code})[code]
    mainPctMap := map[string]float64{}
    {
        // 优先 DB 读取
        var mrows []*model.SharesDailyTbl
        _ = model.SharesDailyTblMgr(core.Dao.GetDBr().Where("code = ?", code).Order("day0 desc").Limit(req.Days+2)).Find(&mrows).Error
        if len(mrows) > 0 {
            for _, r := range mrows { if strings.TrimSpace(r.Day0Str) != "" { mainPctMap[r.Day0Str] = r.Percent } }
        } else {
            // 回退日K
            kd, err := event.GetDayliy(code)
            if err == nil && len(kd) > 0 {
                for i := 1; i < len(kd); i++ {
                    prevClose, _ := kd[i-1][2].(float64)
                    close, _ := kd[i][2].(float64)
                    if prevClose > 0 { ds, _ := kd[i][0].(string); mainPctMap[ds] = (close - prevClose) / prevClose * 100 }
                }
            }
        }
    }
    // 判定函数
    isUp := func(pct float64, code, hy string) bool {
        if mode == "bigrise" {
            // 相对阈值优先
            if minRate > 0 {
                thr := guessLimitRatio(code, hy)*100*minRate
                return pct >= thr - 0.2
            }
            return pct >= minPct - 0.2
        }
        return isLimitUp(pct, code, hy)
    }

    var wg sync.WaitGroup
    sem := make(chan struct{}, 6)
    mu := sync.Mutex{}
    for _, co := range candidates {
        co := co
        wg.Add(1)
        sem <- struct{}{}
        go func(){
            defer func(){ <-sem; wg.Done() }()
            // 1) DB 快速路径
            var rows []*model.SharesDailyTbl
            _ = model.SharesDailyTblMgr(core.Dao.GetDBr().Where("code = ?", co).Order("day0 desc").Limit(req.Days+2)).Find(&rows).Error
            if len(rows) > 0 {
                local := make(map[string]float64)
                for _, r := range rows {
                    if strings.TrimSpace(r.Day0Str) != "" { local[r.Day0Str] = r.Percent }
                }
                mu.Lock()
                for d := range dateSet {
                    if pct, ok := local[d]; ok { if isUp(pct, co, hyMp[co]) { countMp[d] = countMp[d] + 1 } }
                }
                mu.Unlock()
                return
            }
            // 2) 回退：网络日K
            kd, err := event.GetDayliy(co)
            if err != nil || len(kd) == 0 { return }
            // 构造 date->percent
            pctMap := map[string]float64{}
            for i := 1; i < len(kd); i++ {
                prevClose, _ := kd[i-1][2].(float64)
                close, _ := kd[i][2].(float64)
                if prevClose > 0 {
                    pct := (close - prevClose) / prevClose * 100
                    ds, _ := kd[i][0].(string)
                    pctMap[ds] = pct
                }
            }
            mu.Lock()
            for d := range dateSet { if pct, ok := pctMap[d]; ok { if isUp(pct, co, hyMp[co]) { countMp[d] = countMp[d] + 1 } } }
            mu.Unlock()
        }()
    }
    wg.Wait()

    // 构建输出（按日期升序）
    var items []CalendarItem
    for _, d := range baseDates {
        var muFlag bool
        if pct, ok := mainPctMap[d]; ok { muFlag = isUp(pct, code, mainHy) }
        items = append(items, CalendarItem{ Date: d, Count: countMp[d], MainUp: muFlag })
    }
    c.GetGinCtx().JSON(http.StatusOK, &SameConceptLimitupCalendarResp{ Code: code, Concepts: concepts, Codes: len(candidates), Items: items })
}

// SameConceptLimitupDayReq 请求：指定日期同概念涨停明细
type SameConceptLimitupDayReq struct {
    Code       string `json:"code" form:"code"`
    Date       string `json:"date" form:"date"` // YYYY-MM-DD，缺省为今天
    PerConcept int    `json:"perConcept" form:"perConcept"`
    Concepts   []string `json:"concepts" form:"concepts"`
    Mode       string  `json:"mode" form:"mode"`
    MinPct     float64 `json:"minPct" form:"minPct"`
    MinRate    float64 `json:"minRate" form:"minRate"`
}

type SameConceptLimitupDayItem struct {
    Code      string  `json:"code"`
    Name      string  `json:"name"`
    Percent   float64 `json:"percent"`
    FirstSeal string  `json:"firstSeal"` // 当天可用，历史为空
}

type SameConceptLimitupDayResp struct {
    Code  string                         `json:"code"`
    Date  string                         `json:"date"`
    Items []SameConceptLimitupDayItem    `json:"items"`
}

// SameConceptLimitupDay 返回指定日期“同概念涨停”股票列表（今天包含首封时间估算）
func SameConceptLimitupDay(c *api.Context) {
    req := SameConceptLimitupDayReq{ PerConcept: 80 }
    _ = c.GetGinCtx().ShouldBind(&req)
    if strings.TrimSpace(req.Code) == "" {
        c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err":"code required"})
        return
    }
    if req.PerConcept <= 0 { req.PerConcept = 80 }
    mode := strings.ToLower(strings.TrimSpace(req.Mode)); if mode == "" { mode = "limitup" }
    minPct := req.MinPct; if minPct <= 0 { minPct = 7 }
    code := strings.ToLower(strings.TrimSpace(req.Code))
    minRate := req.MinRate

    dateStr := strings.TrimSpace(req.Date)
    if dateStr == "" { dateStr = tools.GetDayStr(time.Now()) }

    concepts := conceptsForCode(code)
    if len(req.Concepts) == 0 {
        raw := c.GetGinCtx().Query("concepts"); if raw == "" { raw = c.GetGinCtx().PostForm("concepts") }
        if strings.TrimSpace(raw) != "" { req.Concepts = splitConceptsFlexible(raw) }
    }
    if len(req.Concepts) > 0 {
        concepts = filterConceptsBySelection(concepts, req.Concepts)
    }
    candSet := make(map[string]struct{})
    for _, name := range concepts {
        for _, co := range codesByConcept(name, req.PerConcept) { if co != "" && !strings.EqualFold(co, code) { candSet[co]=struct{}{} } }
    }
    var codes []string
    for k := range candSet { codes = append(codes, k) }
    if len(codes) == 0 { c.GetGinCtx().JSON(http.StatusOK, &SameConceptLimitupDayResp{ Code: code, Date: dateStr, Items: []SameConceptLimitupDayItem{} }); return }

    hyMp := hyNameByCodes(codes)
    nameMp := namesByCodes(codes)
    today := tools.GetDayStr(time.Now())
    var items []SameConceptLimitupDayItem

    if dateStr == today {
        // 当天：用实时 percent 判断，达到阈值的再拉分钟线估算首封时间
        infos, _ := event.GetShares(codes, true)
        // 建映射
        for _, it := range infos {
            // 是否达到口径
            pass := false
            if mode == "bigrise" {
                if minRate > 0 { pass = it.Percent >= guessLimitRatio(it.Code, hyMp[it.Code])*100*minRate - 0.2 } else { pass = it.Percent >= minPct - 0.2 }
            } else {
                pass = isLimitUp(it.Percent, it.Code, hyMp[it.Code])
            }
            if pass { items = append(items, SameConceptLimitupDayItem{ Code: it.Code, Name: nameMp[it.Code], Percent: round2(it.Percent) }) }
        }
        if mode != "bigrise" { // 仅涨停模式才估算首封时间
            var wg sync.WaitGroup
            sem := make(chan struct{}, 6)
            mu := sync.Mutex{}
            for i := range items {
                i := i
                wg.Add(1)
                sem <- struct{}{}
                go func(){
                    defer func(){ <-sem; wg.Done() }()
                    m, err := event.GetMinute(items[i].Code)
                    if err != nil || m == nil || len(m.List) == 0 || m.PrePrice <= 0 { return }
                    ratio := guessLimitRatio(items[i].Code, hyMp[items[i].Code])
                    limitPrice := m.PrePrice * (1 + ratio)
                    var first string
                    for _, v := range m.List { if v.Price >= limitPrice - 1e-6 { first = v.Time; break } }
                    if first != "" { mu.Lock(); items[i].FirstSeal = first; mu.Unlock() }
                }()
            }
            wg.Wait()
        }
        sort.Slice(items, func(i,j int) bool {
            if items[i].FirstSeal != "" && items[j].FirstSeal != "" { return items[i].FirstSeal < items[j].FirstSeal }
            if items[i].FirstSeal != "" { return true }
            if items[j].FirstSeal != "" { return false }
            return items[i].Percent > items[j].Percent
        })
        c.GetGinCtx().JSON(http.StatusOK, &SameConceptLimitupDayResp{ Code: code, Date: dateStr, Items: items })
        return
    }

    // 历史日：按 shares_daily_tbl 快速判断；缺失则回退网络日K
    // day0 对齐
    parsed, _ := time.Parse("2006-01-02", dateStr)
    day0 := tools.GetUtcDay0(parsed)

    // 批量读取当日 rows
    var rows []*model.SharesDailyTbl
    if len(codes) > 0 {
        _ = model.SharesDailyTblMgr(core.Dao.GetDBr().Where("code in (?) AND day0 = ?", codes, day0)).Find(&rows).Error
    }
    got := make(map[string]*model.SharesDailyTbl)
    for _, r := range rows { got[r.Code] = r }

    // 先使用 DB 命中的
    for _, co := range codes {
        if r, ok := got[co]; ok {
            pass := false
            if mode == "bigrise" {
                if minRate > 0 { pass = r.Percent >= guessLimitRatio(co, hyMp[co])*100*minRate - 0.2 } else { pass = r.Percent >= minPct - 0.2 }
            } else {
                pass = isLimitUp(r.Percent, co, hyMp[co])
            }
            if pass {
                items = append(items, SameConceptLimitupDayItem{ Code: co, Name: nameMp[co], Percent: round2(r.Percent) })
            }
        }
    }

    // 回退：未命中 DB 的代码，走网络日K
    miss := make([]string, 0, len(codes))
    for _, co := range codes { if _, ok := got[co]; !ok { miss = append(miss, co) } }
    var wg sync.WaitGroup
    sem := make(chan struct{}, 6)
    mu := sync.Mutex{}
    for _, co := range miss {
        co := co
        wg.Add(1)
        sem <- struct{}{}
        go func(){
            defer func(){ <-sem; wg.Done() }()
            kd, err := event.GetDayliy(co)
            if err != nil || len(kd) == 0 { return }
            // 找到该 date 的 percent
            var pct *float64
            for i := 1; i < len(kd); i++ {
                ds, _ := kd[i][0].(string)
                if ds != dateStr { continue }
                prevClose, _ := kd[i-1][2].(float64)
                close, _ := kd[i][2].(float64)
                if prevClose > 0 {
                    v := (close - prevClose) / prevClose * 100
                    pct = &v
                }
                break
            }
            if pct == nil { return }
            pass := false
            if mode == "bigrise" {
                if minRate > 0 { pass = *pct >= guessLimitRatio(co, hyMp[co])*100*minRate - 0.2 } else { pass = *pct >= minPct - 0.2 }
            } else {
                pass = isLimitUp(*pct, co, hyMp[co])
            }
            if pass {
                mu.Lock()
                items = append(items, SameConceptLimitupDayItem{ Code: co, Name: nameMp[co], Percent: round2(*pct) })
                mu.Unlock()
            }
        }()
    }
    wg.Wait()

    sort.Slice(items, func(i,j int) bool { return items[i].Percent > items[j].Percent })
    c.GetGinCtx().JSON(http.StatusOK, &SameConceptLimitupDayResp{ Code: code, Date: dateStr, Items: items })
}

// helpers

// conceptsForCode 按代码返回概念（结构化优先，回退 hy_name）
func conceptsForCode(code string) []string {
    var lst []string
    type row struct{ Name string }
    var rows []row
    core.Dao.GetDBr().Raw("SELECT name FROM concept_map_tbl WHERE code = ? ORDER BY id", code).Scan(&rows)
    for _, r := range rows { if s := strings.TrimSpace(r.Name); s != "" { lst = append(lst, s) } }
    if len(lst) == 0 {
        info, _ := model.SharesInfoTblMgr(core.Dao.GetDBr().DB).GetFromCode(code)
        for _, v := range strings.Split(info.HyName, ",") { v = strings.TrimSpace(v); if v != "" { lst = append(lst, v) } }
    }
    return lst
}

// codesByConcept 返回某概念下的股票代码（优先结构化，回退 hy_name like）
func codesByConcept(name string, limit int) []string {
    name = canonicalizeConcept(name)
    type row struct{ Code string }
    var rows []row
    core.Dao.GetDBr().Raw("SELECT code FROM concept_map_tbl WHERE name = ? LIMIT ?", name, limit*5).Scan(&rows)
    var codes []string
    for _, r := range rows { if r.Code != "" { codes = append(codes, strings.ToLower(r.Code)) } }
    if len(codes) == 0 {
        like := "%" + name + "%"
        var infos []*model.SharesInfoTbl
        _ = model.SharesInfoTblMgr(core.Dao.GetDBr().Where("hy_name LIKE ?", like).Limit(limit*5)).Find(&infos).Error
        for _, v := range infos { codes = append(codes, strings.ToLower(v.Code)) }
    }
    return unique(codes)
}

func unique(in []string) []string {
    seen := map[string]struct{}{}
    var out []string
    for _, v := range in { v = strings.ToLower(strings.TrimSpace(v)); if v=="" {continue}; if _,ok:=seen[v]; ok {continue}; seen[v]=struct{}{}; out=append(out, v) }
    return out
}

func hyNameByCodes(codes []string) map[string]string {
    mp := make(map[string]string, len(codes))
    if len(codes) == 0 { return mp }
    var infos []*model.SharesInfoTbl
    _ = model.SharesInfoTblMgr(core.Dao.GetDBr().Where("code in (?)", codes)).Find(&infos).Error
    for _, s := range infos { mp[s.Code] = s.HyName }
    return mp
}

// lastTradingDaysByCode 返回主股票最近 N 个交易日（按 shares_daily_tbl 或网络 K 线）升序 YYYY-MM-DD
func lastTradingDaysByCode(code string, days int) []string {
    var rows []*model.SharesDailyTbl
    _ = model.SharesDailyTblMgr(core.Dao.GetDBr().Where("code = ?", code).Order("day0 desc").Limit(days)).Find(&rows).Error
    if len(rows) > 0 {
        var ds []string
        for _, r := range rows { if s := strings.TrimSpace(r.Day0Str); s != "" { ds = append(ds, s) } }
        // 目前 ds 是倒序，转正序
        for i, j := 0, len(ds)-1; i < j; i, j = i+1, j-1 { ds[i], ds[j] = ds[j], ds[i] }
        return ds
    }
    // 回退
    kd, err := event.GetDayliy(code)
    if err != nil || len(kd) == 0 { return nil }
    var ds []string
    start := 0
    if len(kd) > days { start = len(kd) - days }
    for i := start; i < len(kd); i++ { if s, ok := kd[i][0].(string); ok { ds = append(ds, s) } }
    return ds
}

// splitConceptsFlexible 支持逗号/空白/中文分隔
func splitConceptsFlexible(s string) []string {
    fields := strings.FieldsFunc(s, func(r rune) bool {
        switch r { case ' ', '\t', '\n', '\r', ',', '，', '、', ';', '；', '|': return true }
        return false
    })
    var out []string
    seen := map[string]struct{}{}
    for _, f := range fields { f = strings.TrimSpace(f); if f=="" {continue}; if _,ok:=seen[f]; ok {continue}; seen[f]=struct{}{}; out=append(out, f) }
    return out
}

// filterConceptsBySelection 对主股票概念进行筛选：取与选择集的交集（别名规范化）
func filterConceptsBySelection(all []string, selection []string) []string {
    if len(selection) == 0 { return all }
    sel := map[string]struct{}{}
    for _, v := range selection { v = canonicalizeConcept(v); if strings.TrimSpace(v) != "" { sel[v] = struct{}{} } }
    var out []string
    for _, v := range all { vv := canonicalizeConcept(v); if _, ok := sel[vv]; ok { out = append(out, v) } }
    return out
}
