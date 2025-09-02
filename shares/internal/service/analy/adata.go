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

    // 解析两种格式
    codeToConcepts := make(map[string][]string)
    var byConcept AdataConceptList
    if err := json.Unmarshal(payload, &byConcept); err == nil && len(byConcept.Concepts) > 0 {
        for _, item := range byConcept.Concepts {
            for _, code := range item.Codes {
                code = strings.TrimSpace(strings.ToLower(code))
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
            code := strings.TrimSpace(strings.ToLower(item.Code))
            if code == "" { continue }
            for _, name := range item.Concepts {
                codeToConcepts[code] = append(codeToConcepts[code], strings.TrimSpace(name))
            }
        }
    }

    if len(codeToConcepts) == 0 { return errors.New("adata parsed but empty mapping") }

    orm := core.Dao.GetDBw()
    // Ensure table
    orm.Exec("CREATE TABLE IF NOT EXISTS concept_map_tbl (\n  id int(11) NOT NULL AUTO_INCREMENT,\n  code varchar(255) DEFAULT NULL,\n  hy_code varchar(255) DEFAULT NULL,\n  name varchar(255) DEFAULT NULL,\n  created_at datetime DEFAULT NULL,\n  UNIQUE KEY code_name (code,name),\n  PRIMARY KEY (id)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;")

    // Apply per code
    for code, concepts := range codeToConcepts {
        // 1) 更新结构化映射
        orm.Exec("DELETE FROM concept_map_tbl WHERE code = ?", code)
        for _, name := range concepts {
            if name == "" { continue }
            orm.Exec("INSERT INTO concept_map_tbl(code,name,created_at) VALUES(?,?,?)", code, name, time.Now())
        }
        // 2) 更新 shares_info_tbl.hy_name 以便兼容旧接口
        hy := strings.Join(filterNonEmpty(concepts), ",")
        model.SharesInfoTblMgr(orm.Where("code = ?", code)).Update(model.SharesInfoTblColumns.HyName, hy)
    }

    return nil
}

func filterNonEmpty(in []string) (out []string) {
    for _, v := range in { v = strings.TrimSpace(v); if v != "" { out = append(out, v) } }
    return
}
