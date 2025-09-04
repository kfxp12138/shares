package analy

import (
    "net/http"
    "sort"
    "strings"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
    "shares/internal/service/event"
    proto "shares/rpc/shares"
)

// PickCodesReq 以代码集为输入的榜单请求
type PickCodesReq struct {
    Codes  []string `json:"codes" form:"codes"` // 逗号分隔或数组
    Limit  int      `json:"limit" form:"limit"`
}

// PickCodes 以代码集为输入，返回与 MyBoard 相同结构
func PickCodes(c *api.Context) {
    req := PickCodesReq{Limit: 100}
    // 支持 JSON / 表单 / query
    _ = c.GetGinCtx().ShouldBind(&req)
    if len(req.Codes) == 0 {
        // 尝试从 query/form 的字符串逗号分隔解析
        raw := c.GetGinCtx().Query("codes")
        if raw == "" {
            raw = c.GetGinCtx().PostForm("codes")
        }
        if raw != "" {
            for _, v := range strings.Split(raw, ",") {
                if s := strings.TrimSpace(v); s != "" { req.Codes = append(req.Codes, s) }
            }
        }
    }
    if req.Limit <= 0 { req.Limit = 100 }

    // 归一化代码：如果用户输入 600000 之类，尝试用 Searchs 自动补全
    var norm []string
    for _, code := range req.Codes {
        code = strings.ToLower(strings.TrimSpace(code))
        if code == "" { continue }
        if strings.HasPrefix(code, "sh") || strings.HasPrefix(code, "sz") || strings.HasPrefix(code, "hk") {
            norm = append(norm, code)
        } else {
            if s := event.Searchs([]string{"sh"+code, "sz"+code, "hk"+code}); len(s) > 0 {
                norm = append(norm, s[0].Code)
            } else {
                norm = append(norm, code)
            }
        }
    }
    if len(norm) == 0 {
        c.GetGinCtx().JSON(http.StatusOK, &MyBoardResp{List: []*MyBoardRow{}})
        return
    }

    // 实时基本信息
    basics, _ := event.GetShares(norm, true)
    bMp := map[string]*proto.SharesInfo{}
    var codes []string
    for _, b := range basics { bMp[b.Code] = b; codes = append(codes, b.Code) }

    // 细节
    details := event.SearchDetails(codes)
    dMp := map[string]*proto.SharesInfoDetails{}
    for _, d := range details { dMp[d.Code] = d }

    // 概念聚合：合并 shares_info_tbl.hy_name 与 concept_map_tbl 名称，确保“所有概念”完整返回
    hyMp := map[string]string{}
    if infos, err := model.SharesInfoTblMgr(core.Dao.GetDBr().Where("code in (?)", codes)).Gets(); err == nil {
        for _, s := range infos { hyMp[s.Code] = s.HyName }
    }
    if len(codes) > 0 {
        type cm struct{ Code string; Names string }
        var cms []cm
        // 1) 先按完整代码聚合
        core.Dao.GetDBr().Raw("SELECT code, GROUP_CONCAT(name ORDER BY id SEPARATOR ',') AS names FROM concept_map_tbl WHERE code IN (?) GROUP BY code", codes).Scan(&cms)
        for _, v := range cms {
            if v.Names == "" { continue }
            merged, _ := mergeConcepts(hyMp[v.Code], v.Names)
            hyMp[v.Code] = merged
        }
        // 2) 再按简码聚合（兼容历史导入为裸数字的情况）
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
        core.Dao.GetDBr().Raw("SELECT code, GROUP_CONCAT(name ORDER BY id SEPARATOR ',') AS names FROM concept_map_tbl WHERE code IN (?) GROUP BY code", simples).Scan(&cms2)
        for _, v := range cms2 {
            full := simpleMap[v.Code]
            if full == "" || v.Names == "" { continue }
            merged, _ := mergeConcepts(hyMp[full], v.Names)
            hyMp[full] = merged
        }
    }

    var rows []*MyBoardRow
    for _, code := range codes {
        b := bMp[code]
        if b == nil { continue }

        // 分钟线衍生指标
        var curVol int64
        var speed float64
        var firstSeal string
        if m, err := event.GetMinute(code); err == nil && m != nil && len(m.List) > 0 {
            last := m.List[len(m.List)-1]
            curVol = last.Vol
            if len(m.List) >= 2 && m.PrePrice > 0 {
                prev := m.List[len(m.List)-2]
                curPct := (last.Price - m.PrePrice) / m.PrePrice * 100
                prevPct := (prev.Price - m.PrePrice) / m.PrePrice * 100
                speed = curPct - prevPct
            }
            if m.PrePrice > 0 {
                limit := guessLimitRatio(code, hyMp[code])
                limitPrice := m.PrePrice * (1 + limit)
                for _, v := range m.List {
                    if v.Price >= limitPrice-1e-6 { firstSeal = v.Time; break }
                }
            }
        }

        // 几板（近似）
        boards := 0
        if list, err := model.SharesDailyTblMgr(core.Dao.GetDBr().Where("code = ?", code).Order("day0 desc").Limit(10)).GetFromCode(code); err == nil {
            for _, d := range list { if isLimitUp(d.Percent, code, hyMp[code]) { boards++ } else { break } }
        }
        if isLimitUp(b.Percent, code, hyMp[code]) { boards++ }

        // 换手
        var tor float64
        if d := dMp[code]; d != nil { tor = d.TurnoverRate }

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

    sort.Slice(rows, func(i,j int) bool { return rows[i].Percent > rows[j].Percent })
    if req.Limit < len(rows) { rows = rows[:req.Limit] }
    c.GetGinCtx().JSON(http.StatusOK, &MyBoardResp{List: rows})
}
