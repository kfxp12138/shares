package analy

import "strings"

// splitConceptString 将概念字符串按常见分隔符拆分为去重后的数组
func splitConceptString(s string) []string {
    if strings.TrimSpace(s) == "" { return nil }
    fields := strings.FieldsFunc(s, func(r rune) bool {
        switch r {
        case ' ', '\t', '\n', '\r', ',', '，', '、', ';', '；', '|', '/', '／':
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

// mergeConcepts 合并两段概念字符串，返回合并后的字符串与数组
func mergeConcepts(a, b string) (string, []string) {
    lst := append(splitConceptString(a), splitConceptString(b)...)
    if len(lst) == 0 { return "", nil }
    mp := make(map[string]struct{})
    var out []string
    for _, v := range lst {
        if _, ok := mp[v]; ok { continue }
        mp[v] = struct{}{}
        out = append(out, v)
    }
    return strings.Join(out, ","), out
}

