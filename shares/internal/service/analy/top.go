package analy

import (
    "fmt"
    "net/http"
    "sort"
    "strconv"
    "strings"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
    "shares/internal/service/event"
)

// TopStocks 返回按涨幅降序的股票列表（可选概念过滤）
// GET /analy.top_stocks?limit=200&concept=机器人
func TopStocks(c *api.Context) {
    // params
    limit := 200
    offset := 0
    sortKey := strings.ToLower(strings.TrimSpace(c.GetGinCtx().Query("sort")))
    order := strings.ToLower(strings.TrimSpace(c.GetGinCtx().Query("order")))
    if v := c.GetGinCtx().Query("limit"); v != "" { if n, _ := strconv.Atoi(v); n > 0 { limit = n } }
    if v := c.GetGinCtx().Query("offset"); v != "" { if n, _ := strconv.Atoi(v); n >= 0 { offset = n } }
    if order != "asc" { order = "desc" }
    if sortKey == "" { sortKey = "percent" }
    switch sortKey { case "percent", "price", "code", "name": default: sortKey = "percent" }
    orderBy := fmt.Sprintf("%s %s", sortKey, order)

    concept := strings.TrimSpace(c.GetGinCtx().Query("concept"))
    realtime := strings.TrimSpace(c.GetGinCtx().Query("realtime")) == "1"

    orm := core.Dao.GetDBr()
    db := model.SharesInfoTblMgr(orm.DB)
    if concept != "" { db = model.SharesInfoTblMgr(orm.Where("hy_name LIKE ?", "%"+concept+"%")) }

    var rows []*model.SharesInfoTbl
    // 预先按快照排序拿候选，避免一次性实时拉取过多
    candidate := limit*2 + offset
    if candidate < limit { candidate = limit }
    if realtime {
        _ = db.Order("percent desc").Limit(candidate).Offset(0).Find(&rows).Error
    } else {
        _ = db.Order(orderBy).Limit(limit).Offset(offset).Find(&rows).Error
    }

    type item struct { Code string `json:"code"`; Name string `json:"name"`; Price float64 `json:"price"`; Percent float64 `json:"percent"`; Concepts string `json:"concepts"` }
    var list []item

    if realtime && len(rows) > 0 {
        // 实时拉取候选的行情
        var codes []string
        hyMp := make(map[string]string)
        for _, v := range rows { codes = append(codes, v.Code); hyMp[v.Code] = v.HyName }
        type cm struct{ Code string; Name string }
        var cms []cm
        core.Dao.GetDBr().Raw("SELECT code, GROUP_CONCAT(name) AS name FROM concept_map_tbl WHERE code IN (?) GROUP BY code", codes).Scan(&cms)
        for _, v := range cms { if v.Name != "" { hyMp[v.Code] = v.Name } }
        outs, _ := event.GetShares(codes, true)
        for _, v := range outs {
            list = append(list, item{Code: v.Code, Name: v.Name, Price: v.Price, Percent: v.Percent, Concepts: hyMp[v.Code]})
        }
        // 排序与分页
        sort.Slice(list, func(i,j int) bool {
            switch sortKey {
            case "price":
                if order == "asc" { return list[i].Price < list[j].Price } else { return list[i].Price > list[j].Price }
            case "code":
                if order == "asc" { return list[i].Code < list[j].Code } else { return list[i].Code > list[j].Code }
            case "name":
                if order == "asc" { return list[i].Name < list[j].Name } else { return list[i].Name > list[j].Name }
            default: // percent
                if order == "asc" { return list[i].Percent < list[j].Percent } else { return list[i].Percent > list[j].Percent }
            }
        })
        if offset < len(list) {
            end := offset+limit
            if end > len(list) { end = len(list) }
            list = list[offset:end]
        } else {
            list = []item{}
        }
    } else {
        var codes []string
        hyMp := make(map[string]string)
        for _, v := range rows { codes = append(codes, v.Code); hyMp[v.Code] = v.HyName }
        type cm struct{ Code string; Name string }
        var cms []cm
        core.Dao.GetDBr().Raw("SELECT code, GROUP_CONCAT(name) AS name FROM concept_map_tbl WHERE code IN (?) GROUP BY code", codes).Scan(&cms)
        for _, v := range cms { if v.Name != "" { hyMp[v.Code] = v.Name } }
        for _, v := range rows { list = append(list, item{Code: v.Code, Name: v.Name, Price: v.Price, Percent: v.Percent, Concepts: hyMp[v.Code]}) }
    }
    c.GetGinCtx().JSON(http.StatusOK, map[string]any{"list": list, "limit": limit, "offset": offset, "sort": sortKey, "order": order})
}
