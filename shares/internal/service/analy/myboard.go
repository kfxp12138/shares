package analy

import (
    "math"
    "net/http"
    "sort"
    "strings"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
    "shares/internal/service/event"
    proto "shares/rpc/shares"
)

// MyBoardReq 自选股榜单请求
type MyBoardReq struct {
    // 限制返回条数，默认 100（按涨幅排序）
    Limit int `json:"limit" form:"limit"`
}

// MyBoardRow 自选股榜单行
type MyBoardRow struct {
    Code       string  `json:"code"`
    Name       string  `json:"name"`
    Percent    float64 `json:"percent"`    // 涨幅（%）
    Price      float64 `json:"price"`      // 现价
    Boards     int     `json:"boards"`     // 几板（近端估算）
    FirstSeal  string  `json:"firstSeal"`  // 首封时间（HH:MM），若未触及涨停则为空
    Concepts   string  `json:"concepts"`             // 概念板块（字符串，逗号分隔）
    ConceptsArr []string `json:"conceptsArr,omitempty"` // 概念数组（前端更易用）
    CurVol     int64   `json:"curVol"`     // 现量（最近一分钟成交量）
    Speed      float64 `json:"speed"`      // 涨速（最近一分钟涨幅增量，pct）
    TurnoverRt float64 `json:"turnoverRt"` // 换手率（%）
}

// MyBoardResp 响应
type MyBoardResp struct {
    List []*MyBoardRow `json:"list"`
}

// MyBoard 自选股榜单
// 路由：/analy.my_board（GET/POST）
func MyBoard(c *api.Context) {
    req := MyBoardReq{Limit: 100}
    if err := c.GetGinCtx().ShouldBind(&req); err != nil {
        _ = c.GetGinCtx().ShouldBindJSON(&req)
    }
    if req.Limit <= 0 {
        req.Limit = 100
    }

    // 获取用户
    user, err := c.GetUserInfo()
    if err != nil {
        c.GetGinCtx().JSON(http.StatusUnauthorized, map[string]string{"err": err.Error()})
        return
    }

    ormR := core.Dao.GetDBr()

    // 读取自选代码
    watches, err := model.SharesWatchTblMgr(ormR.Where("open_id = ?", user.Info.Openid).Order("id desc")).Gets()
    if err != nil {
        c.GetGinCtx().JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
        return
    }
    if len(watches) == 0 {
        c.GetGinCtx().JSON(http.StatusOK, &MyBoardResp{List: []*MyBoardRow{}})
        return
    }

    // 代码集合
    var codes []string
    for _, w := range watches {
        codes = append(codes, w.Code)
    }

    // 实时基本信息（价格/涨幅）
    basics, _ := event.GetShares(codes, user.Info.Rg)
    basicMp := map[string]*proto.SharesInfo{}
    for _, b := range basics {
        // event.GetShares 返回 *proto.SharesInfo，但在此仅取字段，按接口抽象
        // 为避免引入 rpc 包，定义轻量引用：通过字段复制
        basicMp[b.Code] = b
    }

    // 明细（换手率等）
    details := event.SearchDetails(codes)
    detailMp := map[string]*proto.SharesInfoDetails{}
    for _, d := range details {
        detailMp[d.Code] = d
    }

    // 代码 -> 概念（优先 concept_map_tbl，其次 hy_name）
    hyMp := map[string]string{}
    if infos, err := model.SharesInfoTblMgr(ormR.Where("code in (?)", codes)).Gets(); err == nil {
        for _, s := range infos { hyMp[s.Code] = s.HyName }
    }
    type cm struct{ Code string; Names string }
    var cms []cm
    // 1) 完整代码
    ormR.Raw("SELECT code, GROUP_CONCAT(name ORDER BY id SEPARATOR ',') AS names FROM concept_map_tbl WHERE code IN (?) GROUP BY code", codes).Scan(&cms)
    for _, v := range cms {
        if v.Names == "" { continue }
        merged, _ := mergeConcepts(hyMp[v.Code], v.Names)
        hyMp[v.Code] = merged
    }
    // 2) 简码（兼容历史）
    simpleMap := map[string]string{}
    var simples []string
    for _, full := range codes {
        s := full
        if strings.HasPrefix(s, "sh") || strings.HasPrefix(s, "sz") || strings.HasPrefix(s, "hk") || strings.HasPrefix(s, "bj") {
            s = s[2:]
        }
        simpleMap[s] = full
        simples = append(simples, s)
    }
    var cms2 []cm
    ormR.Raw("SELECT code, GROUP_CONCAT(name ORDER BY id SEPARATOR ',') AS names FROM concept_map_tbl WHERE code IN (?) GROUP BY code", simples).Scan(&cms2)
    for _, v := range cms2 {
        full := simpleMap[v.Code]
        if full == "" || v.Names == "" { continue }
        merged, _ := mergeConcepts(hyMp[full], v.Names)
        hyMp[full] = merged
    }

    // 计算榜单
    var rows []*MyBoardRow
    for _, code := range codes {
        b := basicMp[code]
        if b == nil {
            continue
        }

        // 现量/涨速/首封时间 通过分钟线计算
        var curVol int64
        var speed float64
        var firstSeal string
        if m, err := event.GetMinute(code); err == nil && m != nil && len(m.List) > 0 {
            last := m.List[len(m.List)-1]
            curVol = last.Vol
            if len(m.List) >= 2 {
                prev := m.List[len(m.List)-2]
                // 以昨收为基准的涨幅增量
                if m.PrePrice > 0 {
                    curPct := (last.Price - m.PrePrice) / m.PrePrice * 100
                    prevPct := (prev.Price - m.PrePrice) / m.PrePrice * 100
                    speed = curPct - prevPct
                }
            }
            // 估算涨停价与首封时间
            if m.PrePrice > 0 {
                limitRatio := guessLimitRatio(code, hyMp[code])
                limitPrice := m.PrePrice * (1 + limitRatio)
                // 找到第一条 >= 涨停价的分钟
                for _, v := range m.List {
                    if v.Price >= limitPrice-1e-6 {
                        firstSeal = v.Time
                        break
                    }
                }
            }
        }

        // 估算“几板”：基于日线 percent 连板（含当日实时）
        boards := 0
        if list, err := model.SharesDailyTblMgr(ormR.Where("code = ?", code).Order("day0 desc").Limit(10)).GetFromCode(code); err == nil {
            // 连续涨停天数（已收盘）
            for _, d := range list {
                if isLimitUp(d.Percent, code, hyMp[code]) {
                    boards++
                } else {
                    break
                }
            }
        }
        // 当日实时是否涨停（近似）
        if isLimitUp(b.Percent, code, hyMp[code]) {
            boards++
        }

        // 换手率
        var tor float64
        if d := detailMp[code]; d != nil {
            tor = d.TurnoverRate
        }

        // 合并概念：hy_name 与 concept_map_tbl
        mergedStr, mergedArr := mergeConcepts(hyMp[code], "")
        rows = append(rows, &MyBoardRow{
            Code:       b.Code,
            Name:       b.Name,
            Percent:    round2(b.Percent),
            Price:      round2(b.Price),
            Boards:     boards,
            FirstSeal:  firstSeal,
            Concepts:   mergedStr,
            ConceptsArr: mergedArr,
            CurVol:     curVol,
            Speed:      round2(speed),
            TurnoverRt: round2(tor),
        })
    }

    // 排序：按涨幅降序
    sort.Slice(rows, func(i, j int) bool { return rows[i].Percent > rows[j].Percent })
    if req.Limit < len(rows) {
        rows = rows[:req.Limit]
    }

    c.GetGinCtx().JSON(http.StatusOK, &MyBoardResp{List: rows})
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// 估算涨停阈值（简化）：
// - ST: 5%
// - 创业板（sz300开头）、科创板（sh688开头）: 20%
// - 其它: 10%
func guessLimitRatio(code, hy string) float64 {
    if strings.Contains(strings.ToUpper(hy), "ST") { // 概念里带 ST（兜底）
        return 0.05
    }
    if strings.HasPrefix(code, "sz300") || strings.HasPrefix(code, "sh688") {
        return 0.20
    }
    // 代码里直接带 ST 的概率较低，但也尝试一次
    if strings.Contains(strings.ToUpper(code), "ST") {
        return 0.05
    }
    return 0.10
}

func isLimitUp(percent float64, code, hy string) bool {
    // 以 percent 近似判断，允许 0.2 个点的误差
    ratio := guessLimitRatio(code, hy)
    th := ratio*100 - 0.2
    return percent >= th
}
