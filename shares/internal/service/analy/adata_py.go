package analy

import (
    "bytes"
    "context"
    "net/http"
    "os/exec"
    "time"

    "shares/internal/api"
    "shares/internal/config"
)

// RefreshConceptsByPython 调用配置的 Python 脚本生成概念 JSON 并导入
// 仅在开发模式生效（config.is_dev=true），避免生产误调用
// 路由：GET/POST /analy.refresh_concepts_py
func RefreshConceptsByPython(c *api.Context) {
    if !config.GetIsDev() {
        c.GetGinCtx().JSON(http.StatusForbidden, map[string]any{"err": "disabled in non-dev"})
        return
    }
    py := config.GetAdataPython()
    sc := config.GetAdataScript()
    args := config.GetAdataArgs()
    if py == "" || sc == "" {
        c.GetGinCtx().JSON(http.StatusBadRequest, map[string]any{"err": "adata.python or adata.script not set in config"})
        return
    }
    // 允许通过 query 覆盖 script（可选）
    if v := c.GetGinCtx().Query("script"); v != "" { sc = v }

    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx, py, append([]string{sc}, args...)...)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        c.GetGinCtx().JSON(http.StatusInternalServerError, map[string]any{"err": err.Error(), "stderr": stderr.String()})
        return
    }
    payload := stdout.Bytes()
    if len(payload) == 0 {
        c.GetGinCtx().JSON(http.StatusInternalServerError, map[string]any{"err": "python returned empty stdout"})
        return
    }
    if err := refreshConceptsFromJSON(payload); err != nil {
        c.GetGinCtx().JSON(http.StatusInternalServerError, map[string]any{"err": err.Error()})
        return
    }
    c.GetGinCtx().JSON(http.StatusOK, map[string]any{"status": "ok", "bytes": len(payload)})
}

