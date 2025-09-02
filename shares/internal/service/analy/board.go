package analy

import (
    "net/http"
    "sort"
    "strings"
    "time"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
    "shares/internal/service/event"

    "github.com/xxjwxc/public/mylog"
    "github.com/xxjwxc/public/tools"
    "gorm.io/datatypes"
)

// PickBoardReq 请求参数
type PickBoardReq struct {
    // 返回板块数量，默认 30
    Limit int `json:"limit" form:"limit"`
    // 每个板块返回领涨股数量，默认 1
    Leaders int `json:"leaders" form:"leaders"`
}

// BoardStock 领涨股信息
type BoardStock struct {
    Code    string  `json:"code"`
    Name    string  `json:"name"`
    Price   float64 `json:"price"`
    Percent float64 `json:"percent"`
}

// BoardRow 单个板块概览
type BoardRow struct {
    HyCode       string        `json:"hyCode"`
    HyName       string        `json:"hyName"`
    Percent      float64       `json:"percent"`       // 板块涨幅（来自 HyUpTbl）
    TurnoverRate float64       `json:"turnoverRate"`  // 板块换手率
    Num          int           `json:"num"`           // 总家数
    Up           int           `json:"up"`            // 上涨家数
    Zljlr        float64       `json:"zljlr"`         // 当日主力净流入（万元）
    Leaders      []*BoardStock `json:"leaders"`       // 领涨股
}

// PickBoardResp 响应
type PickBoardResp struct {
    List []*BoardRow `json:"list"`
}

// PickBoard 选股板块（热板）聚合
// 注册路由见 routers.InitObj -> "/analy.pick_board"
func PickBoard(c *api.Context) {
    req := PickBoardReq{Limit: 30, Leaders: 1}
    // 支持 query 或 json 两种传参
    if err := c.GetGinCtx().ShouldBind(&req); err != nil {
        _ = c.GetGinCtx().ShouldBindJSON(&req)
    }
    if req.Limit <= 0 {
        req.Limit = 30
    }
    if req.Leaders <= 0 {
        req.Leaders = 1
    }

    ormR := core.Dao.GetDBr()
    ormW := core.Dao.GetDBw()

    // 今日板块热度（HyUpTbl）
    day := datatypes.Date(time.Now())
    hyList, _ := model.HyUpTblMgr(ormR.Where("day = ?", day).Order("percent desc").Limit(req.Limit)).Gets()
    if len(hyList) == 0 { // 尝试即时刷新一次
        watchZTB()
        hyList, _ = model.HyUpTblMgr(ormR.Where("day = ?", day).Order("percent desc").Limit(req.Limit)).Gets()
    }

    // 快速映射 hy -> zljlr（来自 hy_daily_tbl 当天记录）
    day0 := tools.GetUtcDay0(time.Now())
    zljlrMap := map[string]float64{}
    // 批量拉取当日 hy_daily_tbl
    if list, err := model.HyDailyTblMgr(ormR.Where("day0 = ?", day0)).Gets(); err == nil {
        for _, v := range list {
            zljlrMap[v.HyCode] = v.Zljlr
        }
    }

    var out []*BoardRow
    for _, hy := range hyList {
        row := &BoardRow{
            HyCode:       hy.Code,
            HyName:       hy.Name,
            Percent:      hy.Percent,
            TurnoverRate: hy.TurnoverRate,
            Num:          hy.Num,
            Up:           hy.Up,
            Zljlr:        zljlrMap[hy.Code],
        }

        // 取该板块下的候选股票，优先按实时涨幅排序
        // 候选集合：shares_info_tbl.hy_name LIKE %板块名%
        // 取前 50 个做实时刷新以控制开销
        cond := model.Condition{}
        cond.And(model.SharesInfoTblColumns.HyName, "like", "%"+hy.Name+"%")
        where, args := cond.Get()
        var codes []string
        if list, err := model.SharesInfoTblMgr(ormR.Where(where, args...).Limit(50)).Gets(); err == nil {
            for _, v := range list {
                codes = append(codes, v.Code)
            }
        }
        if len(codes) > 0 {
            infos, err := event.GetShares(codes, true)
            if err != nil {
                mylog.Error(err)
            } else {
                sort.Slice(infos, func(i, j int) bool { return infos[i].Percent > infos[j].Percent })
                top := req.Leaders
                if top > len(infos) {
                    top = len(infos)
                }
                for i := 0; i < top; i++ {
                    row.Leaders = append(row.Leaders, &BoardStock{
                        Code:    infos[i].Code,
                        Name:    infos[i].Name,
                        Price:   infos[i].Price,
                        Percent: infos[i].Percent,
                    })
                }
            }
        }

        out = append(out, row)
    }

    c.GetGinCtx().JSON(http.StatusOK, &PickBoardResp{List: out})

    // 轻量的异步：补充行业名称映射等异常情况
    _ = ormW // 保留引用，后续可扩展异步修复
    _ = strings.Builder{}
}
