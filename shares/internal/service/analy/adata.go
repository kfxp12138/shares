package analy

import (
    "encoding/json"
    "errors"
    "fmt"
    "strings"
    "time"

    "shares/internal/api"
    "shares/internal/core"
    "shares/internal/model"
    "github.com/xxjwxc/public/myhttp"
)

// 支持的 adata 载入格式（两种其一，或顶层 concepts / codes）
// 1) { "concepts": [ { "name": "人工智能", "codes": ["sh600000","sz000001"] }, ... ] }
type AdataConceptList struct {
    Concepts []struct {
        Name  string   `json:"name"`
        Codes []string `json:"codes"`
    } `json:"concepts"`
}

// 2) { "codes": [ { "code": "sh600000", "concepts": ["人工智能","CPO"] }, ... ] }
type AdataCodeList struct {
    Codes []struct {
        Code     string   `json:"code"`
        Concepts []string `json:"concepts"`
    } `json:"codes"`
}

// refreshConceptsFromAdata 从外部 adata 源刷新概念映射
// 来源优先级：query:url -> POST body（json） -> query:file(本地路径)
func refreshConceptsFromAdata(c *api.Context) error {
    var payload []byte
    if u := c.GetGinCtx().Query("url"); u != "" {
        payload = []byte(myhttp.OnGetJSON(u, ""))
    } else if c.GetGinCtx().Request.Body != nil {
        b, _ := c.GetGinCtx().GetRawData()
        if len(b) > 0 { payload = b }
    }
    if len(payload) == 0 {
        return errors.New("no adata provided: please pass ?url= or POST json body")
    }
    return refreshConceptsFromJSON(payload)
}

// refreshConceptsFromJSON 核心落库逻辑：解析 payload 并更新 concept_* 表与 shares_info_tbl.hy_name
func refreshConceptsFromJSON(payload []byte) error {
    // 解析两种格式
    codeToConcepts := make(map[string][]string)
    var byConcept AdataConceptList
    if err := json.Unmarshal(payload, &byConcept); err == nil && len(byConcept.Concepts) > 0 {
        for _, item := range byConcept.Concepts {
            for _, code := range item.Codes {
                code = normalizeCodeMarket(strings.TrimSpace(strings.ToLower(code)))
                if code == "" { continue }
                codeToConcepts[code] = append(codeToConcepts[code], strings.TrimSpace(item.Name))
            }
        }
    } else {
        var byCode AdataCodeList
        if err2 := json.Unmarshal(payload, &byCode); err2 != nil || len(byCode.Codes) == 0 {
            return fmt.Errorf("unsupported adata format: %v; %v", err, err2)
        }
        for _, item := range byCode.Codes {
            code := normalizeCodeMarket(strings.TrimSpace(strings.ToLower(item.Code)))
            if code == "" { continue }
            for _, name := range item.Concepts {
                codeToConcepts[code] = append(codeToConcepts[code], strings.TrimSpace(name))
            }
        }
    }
    if len(codeToConcepts) == 0 { return errors.New("adata parsed but empty mapping") }

    orm := core.Dao.GetDBw()
    // Ensure tables
    orm.Exec("CREATE TABLE IF NOT EXISTS concept_master_tbl (\n  id int(11) NOT NULL AUTO_INCREMENT,\n  hy_code varchar(255) DEFAULT NULL,\n  name varchar(255) NOT NULL,\n  created_at datetime DEFAULT NULL,\n  UNIQUE KEY name (name),\n  PRIMARY KEY (id)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;")
    orm.Exec("CREATE TABLE IF NOT EXISTS concept_map_tbl (\n  id int(11) NOT NULL AUTO_INCREMENT,\n  code varchar(255) DEFAULT NULL,\n  hy_code varchar(255) DEFAULT NULL,\n  concept_id int(11) DEFAULT NULL,\n  name varchar(255) DEFAULT NULL,\n  created_at datetime DEFAULT NULL,\n  UNIQUE KEY code_name (code,name),\n  PRIMARY KEY (id)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;")
    // 兼容：补充缺失列
    orm.Exec("ALTER TABLE concept_map_tbl ADD COLUMN concept_id int(11) NULL")

    // Apply per code
    for code, concepts := range codeToConcepts {
        // 1) 清理旧映射
        orm.Exec("DELETE FROM concept_map_tbl WHERE code = ?", code)
        // 2) 写入 master + map（带 concept_id）
        var norm []string
        for _, raw := range concepts {
            name := canonicalizeConcept(raw)
            if name == "" { continue }
            // upsert master（无 hy_code）
            orm.Exec("INSERT IGNORE INTO concept_master_tbl(name,created_at) VALUES(?,?)", name, time.Now())
            var cid int
            orm.Raw("SELECT id FROM concept_master_tbl WHERE name = ? LIMIT 1", name).Scan(&cid)
            orm.Exec("INSERT INTO concept_map_tbl(code,concept_id,name,created_at) VALUES(?,?,?,?)", code, cid, name, time.Now())
            norm = append(norm, name)
        }
        // 3) 回填 shares_info_tbl.hy_name（兼容旧接口）
        hy := strings.Join(filterNonEmpty(norm), ",")
        model.SharesInfoTblMgr(orm.Where("code = ?", code)).Update(model.SharesInfoTblColumns.HyName, hy)
    }
    return nil
}

// refreshConceptsFromURLString 便于定时任务通过 URL 刷新
func refreshConceptsFromURLString(u string) error {
    if strings.TrimSpace(u) == "" { return errors.New("adata url empty") }
    payload := []byte(myhttp.OnGetJSON(u, ""))
    if len(payload) == 0 { return errors.New("adata url returned empty") }
    return refreshConceptsFromJSON(payload)
}

func filterNonEmpty(in []string) (out []string) {
    for _, v := range in { v = strings.TrimSpace(v); if v != "" { out = append(out, v) } }
    return
}

// normalizeCodeMarket 统一代码格式：
// - 已带市场前缀（sh/sz/bj/hk）则转小写返回
// - 否则按常见规则推断：60/68/90->sh；00/20/30->sz；83/87/43/92->bj
func normalizeCodeMarket(code string) string {
    c := strings.ToLower(strings.TrimSpace(code))
    if c == "" { return c }
    if strings.HasPrefix(c, "sh") || strings.HasPrefix(c, "sz") || strings.HasPrefix(c, "hk") || strings.HasPrefix(c, "bj") {
        return c
    }
    if len(c) >= 2 {
        p2 := c[:2]
        switch p2 {
        case "60", "68", "90":
            return "sh" + c
        case "00", "20", "30":
            return "sz" + c
        case "83", "87", "43", "92":
            return "bj" + c
        }
    }
    return c
}
