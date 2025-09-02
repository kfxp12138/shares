package analy

import (
    "encoding/json"
    "net/http"
    "time"

    "shares/internal/api"
    "shares/internal/core"
)

// UpsetConceptAlias 批量/单条维护概念别名映射（alias -> name）
// 接口：POST /analy.upset_concept_alias  Body: {"aliases":[{"alias":"AI","name":"人工智能"}]}
// 或单条: {"alias":"AI","name":"人工智能"}
func UpsetConceptAlias(c *api.Context) {
    orm := core.Dao.GetDBw()
    orm.Exec("CREATE TABLE IF NOT EXISTS concept_alias_tbl (\n  id int(11) NOT NULL AUTO_INCREMENT,\n  alias varchar(255) NOT NULL,\n  name varchar(255) NOT NULL,\n  created_at datetime DEFAULT NULL,\n  UNIQUE KEY alias (alias),\n  PRIMARY KEY (id)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;")

    type A struct{ Alias, Name string }
    var body struct{ Aliases []A }
    b, _ := c.GetGinCtx().GetRawData()
    _ = json.Unmarshal(b, &body)
    if len(body.Aliases) == 0 {
        var one A
        _ = json.Unmarshal(b, &one)
        if one.Alias != "" { body.Aliases = []A{one} }
    }
    for _, it := range body.Aliases {
        if it.Alias == "" || it.Name == "" { continue }
        orm.Exec("INSERT INTO concept_alias_tbl(alias,name,created_at) VALUES(?,?,?) ON DUPLICATE KEY UPDATE name=VALUES(name)", it.Alias, it.Name, time.Now())
    }
    c.GetGinCtx().JSON(http.StatusOK, map[string]any{"status":"ok","count":len(body.Aliases)})
}

