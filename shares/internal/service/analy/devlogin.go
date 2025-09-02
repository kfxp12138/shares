package analy

import (
    "net/http"
    "time"

    "shares/internal/api"
    "shares/internal/config"
    "shares/internal/core"
    "shares/internal/model"
    "github.com/xxjwxc/public/mycache"
    "github.com/xxjwxc/public/myglobal"
)

// DevLogin 一键登录（开发用，无需微信）
// GET/POST /analy.dev_login?openid=dev_openid&nick=开发者
// 设置 user_token / session_token Cookie，并确保在 wx_userinfo 中存在该用户
func DevLogin(c *api.Context) {
    if !config.GetIsDev() {
        c.GetGinCtx().JSON(http.StatusForbidden, map[string]string{"err": "dev_login disabled in non-dev"})
        return
    }
    openid := c.GetGinCtx().Query("openid")
    if openid == "" {
        openid = "dev_openid"
    }
    nick := c.GetGinCtx().Query("nick")
    if nick == "" {
        nick = "开发者"
    }

    // upsert user
    orm := core.Dao.GetDBw()
    u, _ := model.WxUserinfoMgr(orm.DB).GetFromOpenid(openid)
    if u.ID == 0 {
        u.Openid = openid
        u.NickName = nick
        u.Rg = true
        u.Capacity = config.GetMaxCapacity()
        model.WxUserinfoMgr(orm.DB).Save(&u)
    } else {
        if u.NickName != nick && nick != "" {
            u.NickName = nick
            model.WxUserinfoMgr(orm.DB).Save(&u)
        }
    }

    // create session (dev): cache "session_key" -> store minimal session struct
    sessionID, openID, overdue := saveDevSession(openid)

    maxAge := int(time.Until(time.Unix(overdue, 0)).Seconds())
    if maxAge <= 0 { maxAge = 2 * 60 * 60 }
    c.GetGinCtx().SetCookie(api.UserToken, openID, maxAge, "/", c.GetGinCtx().Request.Host, false, true)
    c.GetGinCtx().SetCookie(api.SessionToken, sessionID, maxAge, "/", c.GetGinCtx().Request.Host, false, true)

    c.GetGinCtx().JSON(http.StatusOK, map[string]interface{}{
        "openId":      openID,
        "sessionId":   sessionID,
        "overdueTime": overdue,
    })
}

// saveDevSession mimics weixin.SaveSessionKey without引入微信依赖
func saveDevSession(openid string) (sessionID, openID string, overdueTime int64) {
    type devSession struct {
        Openid     string `json:"openid"`
        SessionKey string `json:"session_key"`
        Unionid    string `json:"unionid"`
    }
    cache := mycache.NewCache("session_key")
    // generate unique session id
    sessionID = myglobal.GetNode().GetIDStr()
    if cache.IsExist(sessionID) { sessionID = myglobal.GetNode().GetIDStr() }
    openID = openid
    cache.Add(sessionID, devSession{Openid: openid, SessionKey: sessionID}, 2*time.Hour)
    overdueTime = time.Now().Add(2 * time.Hour).Unix()
    return
}
