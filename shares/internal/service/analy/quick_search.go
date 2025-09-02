package analy

import (
    "net/http"
    "strings"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
)

type QuickItem struct {
    Code   string `json:"code"`
    Name   string `json:"name"`
    HyName string `json:"hyName"`
}

type QuickResp struct {
    List []*QuickItem `json:"list"`
}

// QuickSearch 简易搜索（无需登录）：支持名称模糊/代码前缀
// 路径：/analy.quick_search?q=关键词
func QuickSearch(c *api.Context) {
    q := c.GetGinCtx().Query("q")
    if q == "" {
        c.GetGinCtx().JSON(http.StatusOK, &QuickResp{List: []*QuickItem{}})
        return
    }
    like := "%" + strings.TrimSpace(q) + "%"
    orm := core.Dao.GetDBr()
    var out []*model.SharesInfoTbl
    // 名称或代码前缀匹配，最多 10 条
    _ = model.SharesInfoTblMgr(orm.Where("name like ? or code like ?", like, like).Limit(10)).Find(&out).Error
    var list []*QuickItem
    for _, v := range out { list = append(list, &QuickItem{Code: v.Code, Name: v.Name, HyName: v.HyName}) }
    c.GetGinCtx().JSON(http.StatusOK, &QuickResp{List: list})
}

