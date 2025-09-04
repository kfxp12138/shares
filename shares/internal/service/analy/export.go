package analy

import (
    "encoding/json"
    "net/http"
    "strings"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
)

// ExportConcepts 以 adata "codes" 形态导出当前数据库中的概念映射
// GET /analy.export_concepts  -> {"codes":[{"code":"sh600000","concepts":["人工智能","CPO"]},...]}
func ExportConcepts(c *api.Context) {
    orm := core.Dao.GetDBr()

    type row struct{ Code string; Names string }
    var rows []row
    // 优先从 concept_map_tbl 导出
    orm.Raw("SELECT code, GROUP_CONCAT(name ORDER BY id SEPARATOR ',') AS names FROM concept_map_tbl GROUP BY code").Scan(&rows)

    // 如果结构化为空，则从 shares_info_tbl.hy_name 导出
    if len(rows) == 0 {
        var infos []*model.SharesInfoTbl
        _ = model.SharesInfoTblMgr(orm.DB).Find(&infos).Error
        for _, v := range infos {
            rows = append(rows, row{Code: v.Code, Names: v.HyName})
        }
    }

    type codeItem struct{ Code string `json:"code"`; Concepts []string `json:"concepts"` }
    out := struct{ Codes []codeItem `json:"codes"` }{}

    for _, r := range rows {
        concepts := splitConcepts(r.Names)
        if len(concepts) == 0 { continue }
        out.Codes = append(out.Codes, codeItem{Code: r.Code, Concepts: concepts})
    }

    bs, _ := json.Marshal(out)
    c.GetGinCtx().Data(http.StatusOK, "application/json; charset=utf-8", bs)
}

func splitConcepts(s string) []string {
    if len(strings.TrimSpace(s)) == 0 { return nil }
    // 支持英文/中文逗号、顿号、分号、竖线
    fields := strings.FieldsFunc(s, func(r rune) bool {
        switch r {
        case ',', '，', '、', ';', '；', '|':
            return true
        }
        return false
    })
    mp := make(map[string]struct{})
    var out []string
    for _, f := range fields {
        f = strings.TrimSpace(f)
        if f == "" { continue }
        if _, ok := mp[f]; ok { continue }
        mp[f] = struct{}{}
        out = append(out, f)
    }
    return out
}

