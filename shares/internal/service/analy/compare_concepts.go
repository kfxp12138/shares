package analy

import (
    "encoding/csv"
    "net/http"
    "sort"
    "strings"
    "time"
    "strconv"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
)

type CompareConceptsReq struct {
    CodesA     []string `json:"codesA" form:"codesA"`
    CodesB     []string `json:"codesB" form:"codesB"`
    OnlyOverlap bool    `json:"onlyOverlap" form:"onlyOverlap"`
}

type CompareConceptsRow struct {
    Code        string   `json:"code"`
    Name        string   `json:"name"`
    Overlap     int      `json:"overlap"`   // 重叠概念数量（未加权）
    Weighted    int      `json:"weighted"`  // 加权重叠：按 A 中概念频次求和
    Concepts    []string `json:"concepts"`  // 重叠概念名列表
    ConceptsDetail []ConceptCount `json:"conceptsDetail,omitempty"` // 重叠概念 + A 中频次
}

type CompareConceptsResp struct {
    Rows      []CompareConceptsRow   `json:"rows"`
    ConceptsA []ConceptCount         `json:"conceptsA"`
}

type ConceptCount struct { Name string `json:"name"`; Count int `json:"count"` }

// CompareConcepts POST /analy.compare_concepts
// body: { codesA:[], codesB:[], onlyOverlap:true }
func CompareConcepts(c *api.Context) {
    var req CompareConceptsReq
    _ = c.GetGinCtx().ShouldBind(&req)
    if len(req.CodesA) == 0 || len(req.CodesB) == 0 {
        // 支持从表单/字符串导入，逗号/空白分隔
        ra := c.GetGinCtx().PostForm("codesA")
        rb := c.GetGinCtx().PostForm("codesB")
        if ra != "" { req.CodesA = splitCodesFlexible(ra) }
        if rb != "" { req.CodesB = splitCodesFlexible(rb) }
    }
    if len(req.CodesA) == 0 || len(req.CodesB) == 0 {
        c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err":"codesA/codesB required"})
        return
    }
    codesA := resolveCodesFlexible(req.CodesA)
    codesB := resolveCodesFlexible(req.CodesB)
    if len(codesA) == 0 || len(codesB) == 0 {
        c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err":"invalid codes"})
        return
    }

    // 概念映射
    conceptsA := conceptsByCodes(codesA)
    conceptsB := conceptsByCodes(codesB)

    // A 概念集合 + 频次
    setA := map[string]struct{}{}
    freq := map[string]int{}
    for _, lst := range conceptsA {
        seen := map[string]struct{}{}
        for _, v := range lst {
            v = strings.TrimSpace(v)
            if v == "" { continue }
            setA[v] = struct{}{}
            if _, ok := seen[v]; ok { continue }
            seen[v] = struct{}{}
            freq[v]++
        }
    }

    // 名称映射
    nameMp := namesByCodes(append(codesA, codesB...))

    // B 侧重叠
    var rows []CompareConceptsRow
    for _, code := range codesB {
        lst := conceptsB[code]
        var matched []string
        var details []ConceptCount
        weighted := 0
        for _, v := range lst {
            if _, ok := setA[v]; ok {
                matched = append(matched, v)
                cnt := freq[v]
                weighted += cnt
                details = append(details, ConceptCount{Name: v, Count: cnt})
            }
        }
        if len(matched) == 0 && req.OnlyOverlap { continue }
        // 对重叠概念按 A 中频次降序排序
        sort.Slice(details, func(i,j int) bool {
            if details[i].Count != details[j].Count { return details[i].Count > details[j].Count }
            return details[i].Name < details[j].Name
        })
        // 重建 concepts 列表以匹配排序
        matched = matched[:0]
        for _, d := range details { matched = append(matched, d.Name) }
        rows = append(rows, CompareConceptsRow{ Code: code, Name: nameMp[code], Overlap: len(matched), Weighted: weighted, Concepts: matched, ConceptsDetail: details })
    }
    sort.Slice(rows, func(i,j int) bool {
        if rows[i].Weighted != rows[j].Weighted {
            return rows[i].Weighted > rows[j].Weighted
        }
        if rows[i].Overlap != rows[j].Overlap {
            return rows[i].Overlap > rows[j].Overlap
        }
        return rows[i].Code < rows[j].Code
    })

    // 频次列表
    var conceptsAList []ConceptCount
    for k, v := range freq { conceptsAList = append(conceptsAList, ConceptCount{Name: k, Count: v}) }
    sort.Slice(conceptsAList, func(i,j int) bool { return conceptsAList[i].Count > conceptsAList[j].Count })

    c.GetGinCtx().JSON(http.StatusOK, &CompareConceptsResp{ Rows: rows, ConceptsA: conceptsAList })
}

// CompareConceptsExport POST /analy.compare_concepts_export  (returns CSV)
func CompareConceptsExport(c *api.Context) {
    start := time.Now()
    var req CompareConceptsReq
    _ = c.GetGinCtx().ShouldBind(&req)
    if len(req.CodesA) == 0 || len(req.CodesB) == 0 {
        ra := c.GetGinCtx().PostForm("codesA")
        rb := c.GetGinCtx().PostForm("codesB")
        if ra != "" { req.CodesA = splitCodesFlexible(ra) }
        if rb != "" { req.CodesB = splitCodesFlexible(rb) }
    }
    if len(req.CodesA) == 0 || len(req.CodesB) == 0 {
        c.GetGinCtx().JSON(http.StatusBadRequest, map[string]string{"err":"codesA/codesB required"})
        return
    }
    req.OnlyOverlap = false
    // 复用对比
    codesA := resolveCodesFlexible(req.CodesA)
    codesB := resolveCodesFlexible(req.CodesB)
    conceptsA := conceptsByCodes(codesA)
    setA := map[string]struct{}{}
    freqExp := map[string]int{}
    for _, lst := range conceptsA {
        seen := map[string]struct{}{}
        for _, v := range lst {
            if v == "" { continue }
            setA[v] = struct{}{}
            if _, ok := seen[v]; ok { continue }
            seen[v] = struct{}{}
            freqExp[v]++
        }
    }
    conceptsB := conceptsByCodes(codesB)
    nameMp := namesByCodes(codesB)

    // CSV 输出
    c.GetGinCtx().Header("Content-Type", "text/csv; charset=utf-8")
    c.GetGinCtx().Header("Content-Disposition", "attachment; filename=concept_overlap.csv")
    w := csv.NewWriter(c.GetGinCtx().Writer)
    _ = w.Write([]string{"code","name","overlap","weighted","concepts_with_weight"})
    for _, code := range codesB {
        lst := conceptsB[code]
        var matched []string
        var weighted int
        for _, v := range lst {
            if _, ok := setA[v]; ok {
                matched = append(matched, v)
                weighted += 1 // 占位，后续替换为频次
            }
        }
        // 频次拼接 name(count)
        // 概念按 A 中频次降序
        sort.Slice(matched, func(i,j int) bool {
            if freqExp[matched[i]] != freqExp[matched[j]] { return freqExp[matched[i]] > freqExp[matched[j]] }
            return matched[i] < matched[j]
        })
        var withW []string
        for _, v := range matched { withW = append(withW, v+"("+strconvI(freqExp[v])+")") }
        // weighted 改为真实加权
        weighted = 0
        for _, v := range matched { weighted += freqExp[v] }
        _ = w.Write([]string{ code, nameMp[code], strconvI(len(matched)), strconvI(weighted), strings.Join(withW, ";") })
    }
    w.Flush()
    _ = w.Error()
    _ = start // for potential logging
}

func splitCodesFlexible(s string) []string {
    fields := strings.FieldsFunc(s, func(r rune) bool {
        switch r { case ' ', '\t', '\n', '\r', ',', '，', '、', ';', '；', '|': return true }
        return false
    })
    var out []string
    seen := map[string]struct{}{}
    for _, f := range fields { f = strings.TrimSpace(f); if f=="" {continue}; if _,ok:=seen[f]; ok {continue}; seen[f]=struct{}{}; out=append(out, f) }
    return out
}

func normalizeCodes(in []string) []string {
    seen := map[string]struct{}{}
    var out []string
    for _, v := range in {
        v = strings.ToLower(strings.TrimSpace(v))
        if v == "" { continue }
        v = normalizeCodeMarket(v)
        if _, ok := seen[v]; ok { continue }
        seen[v] = struct{}{}
        out = append(out, v)
    }
    return out
}

// resolveCodesFlexible 支持名称/前缀代码/裸数字，名称走模糊匹配到 shares_info_tbl
func resolveCodesFlexible(in []string) []string {
    orm := core.Dao.GetDBr()
    seen := map[string]struct{}{}
    var out []string
    for _, raw := range in {
        s := strings.TrimSpace(raw)
        if s == "" { continue }
        lower := strings.ToLower(s)
        var code string
        // 带前缀
        if len(lower) >= 2 && (strings.HasPrefix(lower, "sh") || strings.HasPrefix(lower, "sz") || strings.HasPrefix(lower, "hk") || strings.HasPrefix(lower, "bj")) {
            code = lower
        } else if len(lower) >= 5 && isDigits(lower) { // 裸数字
            code = normalizeCodeMarket(lower)
        } else { // 名称/模糊
            type row struct{ Code string }
            var r row
            // 优先完全匹配名称，其次模糊
            orm.Raw("SELECT code FROM shares_info_tbl WHERE name = ? LIMIT 1", s).Scan(&r)
            if r.Code == "" {
                like := "%" + s + "%"
                orm.Raw("SELECT code FROM shares_info_tbl WHERE name LIKE ? OR code LIKE ? LIMIT 1", like, like).Scan(&r)
            }
            code = strings.ToLower(strings.TrimSpace(r.Code))
            if code == "" { // 最后再尝试 normalize
                code = normalizeCodeMarket(lower)
            }
        }
        if code == "" { continue }
        if _, ok := seen[code]; ok { continue }
        seen[code] = struct{}{}
        out = append(out, code)
    }
    return out
}

func isDigits(s string) bool {
    for _, r := range s { if r < '0' || r > '9' { return false } }
    return s != ""
}

// conceptsByCodes 返回 code->概念数组，合并 concept_map_tbl 与 hy_name
func conceptsByCodes(codes []string) map[string][]string {
    orm := core.Dao.GetDBr()
    out := map[string][]string{}
    if len(codes) == 0 { return out }

    // 先填充 hy_name
    var infos []*model.SharesInfoTbl
    _ = model.SharesInfoTblMgr(orm.Where("code in (?)", codes)).Find(&infos).Error
    for _, s := range infos { if s.HyName != "" { out[s.Code] = splitConceptString(s.HyName) } }

    // 完整代码映射
    type cm struct{ Code string; Names string }
    var cms []cm
    orm.Raw("SELECT code, GROUP_CONCAT(name ORDER BY id SEPARATOR ',') AS names FROM concept_map_tbl WHERE code IN (?) GROUP BY code", codes).Scan(&cms)
    for _, v := range cms {
        if strings.TrimSpace(v.Names) == "" { continue }
        merged, arr := mergeConcepts(strings.Join(out[v.Code],","), v.Names)
        if merged != "" { out[v.Code] = arr }
    }

    // 简码兼容
    simpleMap := map[string]string{}
    var simples []string
    for _, full := range codes {
        s := full
        if len(s) > 2 && (strings.HasPrefix(s, "sh") || strings.HasPrefix(s, "sz") || strings.HasPrefix(s, "hk") || strings.HasPrefix(s, "bj")) {
            s = s[2:]
        }
        simpleMap[s] = full
        simples = append(simples, s)
    }
    var cms2 []cm
    orm.Raw("SELECT code, GROUP_CONCAT(name ORDER BY id SEPARATOR ',') AS names FROM concept_map_tbl WHERE code IN (?) GROUP BY code", simples).Scan(&cms2)
    for _, v := range cms2 {
        full := simpleMap[v.Code]
        if full == "" || strings.TrimSpace(v.Names)=="" { continue }
        merged, arr := mergeConcepts(strings.Join(out[full],","), v.Names)
        if merged != "" { out[full] = arr }
    }
    return out
}

func namesByCodes(codes []string) map[string]string {
    orm := core.Dao.GetDBr()
    mp := map[string]string{}
    if len(codes) == 0 { return mp }
    var infos []*model.SharesInfoTbl
    _ = model.SharesInfoTblMgr(orm.Where("code in (?)", codes)).Find(&infos).Error
    for _, s := range infos { mp[s.Code] = s.Name }
    return mp
}

func strconvI(i int) string { return strconv.FormatInt(int64(i), 10) }
