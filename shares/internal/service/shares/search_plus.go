package shares

import (
    "net/http"
    "strings"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
    "shares/internal/service/event"
)

// SearchPlus 与 Search 类似，但额外返回结构化 concepts 数组（不修改 proto）
// 路由：/shares.search_plus（POST/GET）
// 入参：{ code: "600000", tag: "daily|min" }
// 返回：{ info: shares.SharesInfo, concepts: [string] }
func SearchPlus(c *api.Context) {
    type reqBody struct{ Code, Tag string }
    var req reqBody
    _ = c.GetGinCtx().ShouldBind(&req)
    if req.Code == "" { c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err": "code required"}); return }

    // 基础信息（不强制登录）
    out := event.TrySearch(req.Code)
    if out == nil { c.GetGinCtx().JSON(http.StatusOK, map[string]interface{}{"info": nil, "concepts": []string{}}); return }

    if req.Tag == "" { req.Tag = "daily" }
    // 构造 img 链接
    out.Img = "/shares/echarts/echarts.html?rg=true&only20=false&tag=" + req.Tag + "&code=" + out.Code

    // concepts: 优先结构化表
    var lst []string
    type row struct{ Name string }
    var rows []row
    core.Dao.GetDBr().Raw("SELECT name FROM concept_map_tbl WHERE code = ? ORDER BY id", out.Code).Scan(&rows)
    for _, r := range rows { if strings.TrimSpace(r.Name) != "" { lst = append(lst, strings.TrimSpace(r.Name)) } }
    if len(lst) == 0 { // 回退 shares_info_tbl.hy_name
        info, _ := model.SharesInfoTblMgr(core.Dao.GetDBr().DB).GetFromCode(out.Code)
        for _, v := range strings.Split(info.HyName, ",") { v = strings.TrimSpace(v); if v != "" { lst = append(lst, v) } }
    }

    c.GetGinCtx().JSON(http.StatusOK, map[string]interface{}{"info": out, "concepts": lst})
}

