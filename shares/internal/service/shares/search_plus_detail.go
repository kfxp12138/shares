package shares

import (
    "net/http"
    "strings"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
    "shares/internal/service/event"
)

// SearchPlusDetail 与 SearchPlus 类似，但 concepts 返回结构化对象数组
// 路由：/shares.search_plus_detail（POST/GET）
// 入参：{ code: "600000", tag: "daily|min" }
// 返回：{ info: shares.SharesInfo, concepts: [ { id, name, hyCode } ] }
func SearchPlusDetail(c *api.Context) {
    type reqBody struct{ Code, Tag string }
    var req reqBody
    _ = c.GetGinCtx().ShouldBind(&req)
    if req.Code == "" { c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err": "code required"}); return }

    out := event.TrySearch(req.Code)
    if out == nil { c.GetGinCtx().JSON(http.StatusOK, map[string]interface{}{"info": nil, "concepts": []any{}}); return }
    if req.Tag == "" { req.Tag = "daily" }
    out.Img = "/shares/echarts/echarts.html?rg=true&only20=false&tag=" + req.Tag + "&code=" + out.Code

    type item struct {
        ID     int    `json:"id"`
        Name   string `json:"name"`
        HyCode string `json:"hyCode"`
    }
    var rows []item
    core.Dao.GetDBr().Raw(
        "SELECT m.id as id, m.name as name, m.hy_code as hy_code FROM concept_map_tbl mp LEFT JOIN concept_master_tbl m ON mp.concept_id = m.id WHERE mp.code = ? ORDER BY m.id",
        out.Code,
    ).Scan(&rows)
    // 回退：若结构化表为空，则从 hy_name 拆分
    if len(rows) == 0 {
        info, _ := model.SharesInfoTblMgr(core.Dao.GetDBr().DB).GetFromCode(out.Code)
        for _, v := range strings.Split(info.HyName, ",") {
            v = strings.TrimSpace(v)
            if v != "" {
                rows = append(rows, item{ID: 0, Name: v, HyCode: ""})
            }
        }
    }

    c.GetGinCtx().JSON(http.StatusOK, map[string]interface{}{"info": out, "concepts": rows})
}

