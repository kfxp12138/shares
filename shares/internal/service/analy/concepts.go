package analy

import (
    "net/http"
    "strings"

    "shares/internal/api"
    "shares/internal/config"
    "shares/internal/core"
    "shares/internal/model"
    "gorm.io/datatypes"
    "time"
    "sort"
    "shares/internal/service/event"
    "strconv"
    "github.com/xxjwxc/public/tools"
)

// RefreshConcepts 触发一次板块/概念拉取并刷新股票-概念映射
// 注意：会根据东财板块列表重建 hy_info_tbl，并回填 shares_info_tbl.hy_name
func RefreshConcepts(c *api.Context) { // 仅支持 adata 来源
    if err := refreshConceptsFromAdata(c); err != nil {
        c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err": err.Error(), "hint": "POST JSON body or use ?url= to provide adata"})
        return
    }
    c.GetGinCtx().JSON(http.StatusOK, map[string]any{"status": "ok", "source": "adata"})
}

// RefreshConceptsNow 立即按配置 URL 刷新（adata.concepts_url）
func RefreshConceptsNow(c *api.Context) {
    url := config.GetAdataConceptsURL()
    if strings.TrimSpace(url) == "" {
        c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err": "adata.concepts_url not set in config"})
        return
    }
    if err := refreshConceptsFromURLString(url); err != nil {
        c.GetGinCtx().JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
        return
    }
    c.GetGinCtx().JSON(http.StatusOK, map[string]any{"status": "ok", "url": url})
}

// ConceptsByCode 返回某代码的概念（从 shares_info_tbl.hy_name 拆分）
func ConceptsByCode(c *api.Context) {
    code := c.GetGinCtx().Query("code")
    if code == "" {
        c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err": "invalid parameter"})
        return
    }
    info, err := model.SharesInfoTblMgr(core.Dao.GetDBr().DB).GetFromCode(code)
    if err != nil {
        c.GetGinCtx().JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
        return
    }
    // 优先读取结构化表 concept_map_tbl；若没有，则回退 hy_name
    var lst []string
    type row struct{ Name string }
    var rows []row
    core.Dao.GetDBr().Raw("SELECT name FROM concept_map_tbl WHERE code = ? ORDER BY id", code).Scan(&rows)
    for _, r := range rows { if strings.TrimSpace(r.Name) != "" { lst = append(lst, strings.TrimSpace(r.Name)) } }
    if len(lst) == 0 && len(info.HyName) > 0 {
        for _, v := range strings.Split(info.HyName, ",") {
            v = strings.TrimSpace(v)
            if v != "" { lst = append(lst, v) }
        }
    }
    c.GetGinCtx().JSON(http.StatusOK, map[string]interface{}{"code": code, "concepts": lst})
}

// canonicalizeConcept 根据别名表归一化概念名称
func canonicalizeConcept(name string) string {
    n := strings.TrimSpace(name)
    if n == "" { return n }
    type row struct{ Name string }
    var r row
    core.Dao.GetDBr().Raw("SELECT name FROM concept_alias_tbl WHERE alias = ? LIMIT 1", n).Scan(&r)
    if strings.TrimSpace(r.Name) != "" { return strings.TrimSpace(r.Name) }
    return n
}

// SearchConcepts 按名称关键词搜索板块（来自 hy_info_tbl），可选返回部分成员股票
func SearchConcepts(c *api.Context) {
    q := c.GetGinCtx().Query("q")
    limit := 20
    if q == "" {
        c.GetGinCtx().JSON(http.StatusOK, map[string]interface{}{"list": []interface{}{}})
        return
    }
    like := "%" + q + "%"
    var hys []*model.HyInfoTbl
    orm := core.Dao.GetDBr()
    _ = model.HyInfoTblMgr(orm.Where("name like ?", like).Limit(limit)).Find(&hys).Error
    c.GetGinCtx().JSON(http.StatusOK, map[string]interface{}{"list": hys})
}

// ConceptsDetailByCode 返回结构化概念对象数组：[{id,name,hyCode}]
func ConceptsDetailByCode(c *api.Context) {
    code := c.GetGinCtx().Query("code")
    if code == "" {
        c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err": "invalid parameter"})
        return
    }
    type item struct {
        ID     int    `json:"id"`
        Name   string `json:"name"`
        HyCode string `json:"hyCode"`
    }
    var rows []item
    core.Dao.GetDBr().Raw(
        "SELECT m.id as id, m.name as name, m.hy_code as hy_code FROM concept_map_tbl mp LEFT JOIN concept_master_tbl m ON mp.concept_id = m.id WHERE mp.code = ? ORDER BY m.id",
        code,
    ).Scan(&rows)
    if len(rows) == 0 {
        // 回退：hy_name
        info, _ := model.SharesInfoTblMgr(core.Dao.GetDBr().DB).GetFromCode(code)
        for _, v := range strings.Split(info.HyName, ",") {
            v = strings.TrimSpace(v)
            if v != "" { rows = append(rows, item{ID: 0, Name: v}) }
        }
    }
    c.GetGinCtx().JSON(http.StatusOK, map[string]any{"code": code, "concepts": rows})
}

// ConceptsOverview 当日概念/板块总览
// 返回：[{name, hyCode, percent, turnoverRate, up, num, zljlr, mapNum}]
func ConceptsOverview(c *api.Context) {
    type item struct {
        Name         string  `json:"name"`
        HyCode       string  `json:"hyCode"`
        Percent      float64 `json:"percent"`
        TurnoverRate float64 `json:"turnoverRate"`
        Up           int     `json:"up"`
        Num          int     `json:"num"`
        Zljlr        float64 `json:"zljlr"`
        MapNum       int     `json:"mapNum"`
    }
    day := datatypes.Date(time.Now())
    day0 := tools.GetUtcDay0(time.Now())
    orm := core.Dao.GetDBr()
    // 基于 concept_master_tbl 左联 hy_up_tbl（今日），并聚合 concept_map_tbl 数量、hy_daily_tbl.zljlr
    var list []item
    orm.Raw(`
      SELECT m.name AS name, m.hy_code AS hy_code,
             IFNULL(u.percent,0) AS percent, IFNULL(u.turnover_rate,0) AS turnover_rate,
             IFNULL(u.up,0) AS up, IFNULL(u.num,0) AS num,
             (SELECT COALESCE(SUM(zljlr),0) FROM hy_daily_tbl d WHERE d.hy_code = m.hy_code AND d.day0 = ?) AS zljlr,
             (SELECT COUNT(1) FROM concept_map_tbl mp WHERE mp.name = m.name) AS map_num
      FROM concept_master_tbl m
      LEFT JOIN hy_up_tbl u ON u.code = m.hy_code AND u.day = ?
      ORDER BY percent DESC, zljlr DESC
    `, day0, day).Scan(&list)
    c.GetGinCtx().JSON(http.StatusOK, map[string]any{"list": list})
}

// ConceptStocks 概念成分股（按实时涨幅倒序）
// 入参：name, limit（默认50）
func ConceptStocks(c *api.Context) {
    name := c.GetGinCtx().Query("name")
    if name == "" { c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err":"name required"}); return }
    limit := 50
    if v := c.GetGinCtx().Query("limit"); v != "" {
        if n, _ := strconv.Atoi(v); n > 0 { limit = n }
    }
    name = canonicalizeConcept(name)

    // 先用结构化映射拿 codes
    type row struct{ Code string }
    var rows []row
    core.Dao.GetDBr().Raw("SELECT code FROM concept_map_tbl WHERE name = ? LIMIT ?", name, limit*5).Scan(&rows)
    var codes []string
    for _, r := range rows { if r.Code != "" { codes = append(codes, r.Code) } }
    if len(codes) == 0 { // 回退 hy_name LIKE
        cond := model.Condition{}
        cond.And(model.SharesInfoTblColumns.HyName, "like", "%"+name+"%")
        where, args := cond.Get()
        var infos []*model.SharesInfoTbl
        _ = model.SharesInfoTblMgr(core.Dao.GetDBr().Where(where, args...).Limit(limit*5)).Find(&infos).Error
        for _, v := range infos { codes = append(codes, v.Code) }
    }
    if len(codes) == 0 { c.GetGinCtx().JSON(http.StatusOK, map[string]any{"list": []any{}}); return }

    // 拉实时
    outs, _ := event.GetShares(codes, true)
    type stock struct {
        Code    string  `json:"code"`
        Name    string  `json:"name"`
        Price   float64 `json:"price"`
        Percent float64 `json:"percent"`
    }
    var list []stock
    for _, v := range outs { list = append(list, stock{Code: v.Code, Name: v.Name, Price: v.Price, Percent: v.Percent}) }
    sort.Slice(list, func(i,j int) bool { return list[i].Percent > list[j].Percent })
    if len(list) > limit { list = list[:limit] }
    c.GetGinCtx().JSON(http.StatusOK, map[string]any{"name": name, "list": list})
}
