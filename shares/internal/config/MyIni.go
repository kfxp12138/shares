package config

import (
	"fmt"
	"net/http"
	"strings"
)

// Config custom config struct
type Config struct {
	CfgBase     `yaml:"base"`
	MySQLInfo   MysqlDbInfo    `yaml:"db_info"`
	EtcdInfo    EtcdInfo       `yaml:"etcd_info"`
	RedisDbInfo RedisDbInfo    `yaml:"redis_info"`
	Oauth2Url   string         `yaml:"oauth2_url"`
	Port        string         `yaml:"port"` // 端口号
	WxInfo      WxInfo         `yaml:"wx_info"`
	FileHost    string         `yaml:"file_host"`
	MaxCapacity int            `yaml:"max_capacity"` // 最大容量
	DefGroup    string         `yaml:"def_group"`    // 默认分组
	Ext         []string       `yaml:"ext"`
	ToolsType   int            `yaml:"tools_type"`
	Adata       AdataConfig    `yaml:"adata"`
	Security    SecurityConfig `yaml:"security"`
}

// MysqlDbInfo mysql database information. mysql 数据库信息
type MysqlDbInfo struct {
	Host     string `validate:"required"` // Host. 地址
	Port     int    `validate:"required"` // Port 端口号
	Username string `validate:"required"` // Username 用户名
	Password string // Password 密码
	Database string `validate:"required"` // Database 数据库名
	Type     int    // 数据库类型: 0:mysql , 1:sqlite , 2:mssql
}

// RedisDbInfo redis database information. redis 数据库信息
type RedisDbInfo struct {
	Addrs     []string `yaml:"addrs"` // Host. 地址
	Password  string   // Password 密码
	GroupName string   `yaml:"group_name"` // 分组名字
	DB        int      `yaml:"db"`         // 数据库序号
}

// EtcdInfo etcd config info
type EtcdInfo struct {
	Addrs   []string `yaml:"addrs"`   // Host. 地址
	Timeout int      `yaml:"timeout"` // 超时时间(秒)
}

// WxInfo 微信相关配置
type WxInfo struct {
	AppID     string `yaml:"app_id"`     // 微信公众平台应用ID
	AppSecret string `yaml:"app_secret"` // 微信支付商户平台商户号
	APIKey    string `yaml:"api_key"`    // 微信支付商户平台API密钥
	MchID     string `yaml:"mch_id"`     // 商户号
	NotifyURL string `yaml:"notify_url"` // 通知地址
	ShearURL  string `yaml:"shear_url"`  // 微信分享url
}

// AdataConfig 概念（概念/板块）外部数据源配置
type AdataConfig struct {
	ConceptsURL string   `yaml:"concepts_url"` // 概念映射 JSON 地址（见 VISUAL_API 中两种格式）
	Python      string   `yaml:"python"`       // Python 解释器（例如 python3）
	Script      string   `yaml:"script"`       // 生成概念 JSON 的脚本路径
	Args        []string `yaml:"args"`         // 脚本参数（可选）
}

// SecurityConfig 安全相关配置
type SecurityConfig struct {
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	CookieSecure     *bool    `yaml:"cookie_secure"`
	CookieSameSite   string   `yaml:"cookie_same_site"`
	CookieDomain     string   `yaml:"cookie_domain"`
	EnableMetrics    bool     `yaml:"enable_metrics"`
}

// GetWxInfo 获取微信配置
func GetWxInfo() WxInfo {
	return _map.WxInfo
}

// SetMysqlDbInfo Update MySQL configuration information
func SetMysqlDbInfo(info *MysqlDbInfo) {
	_map.MySQLInfo = *info
}

// GetMysqlDbInfo Get MySQL configuration information .获取mysql配置信息
func GetMysqlDbInfo() MysqlDbInfo {
	return _map.MySQLInfo
}

// GetMysqlConStr Get MySQL connection string.获取mysql 连接字符串
func GetMysqlConStr() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8&parseTime=True&loc=Local&interpolateParams=True",
		_map.MySQLInfo.Username,
		_map.MySQLInfo.Password,
		_map.MySQLInfo.Host,
		_map.MySQLInfo.Port,
		_map.MySQLInfo.Database,
	)
}

// GetCheckTokenURL checktoken
func GetCheckTokenURL() string {
	return _map.Oauth2Url + "/check_token"
}

// GetLoginURL 登陆用的url
func GetLoginURL() string {
	return _map.Oauth2Url + "/authorize"
}

// GetLoginNoPwdURL token 授权模式登陆
func GetLoginNoPwdURL() string {
	return _map.Oauth2Url + "/authorize_nopwd"
}

// GetPort 获取端口号
func GetPort() string {
	return _map.Port
}

// GetRedisDbInfo Get redis configuration information .获取redis配置信息
func GetRedisDbInfo() RedisDbInfo {
	return _map.RedisDbInfo
}

// GetEtcdInfo get etcd configuration information. 获取etcd 配置信息
func GetEtcdInfo() EtcdInfo {
	return _map.EtcdInfo
}

// GetFileHost 获取文件host
func GetFileHost() string {
	return _map.FileHost
}

func GetMaxCapacity() int {
	return _map.MaxCapacity
}

func GetDefGroup() string {
	return _map.DefGroup
}

func GetExt() []string {
	return _map.Ext
}

func GetIsTools() int {
	return _map.ToolsType
}

// GetAdataConceptsURL 获取 adata 概念映射 URL
func GetAdataConceptsURL() string {
	return _map.Adata.ConceptsURL
}

// GetAdataPython 返回 python 配置
func GetAdataPython() string { return _map.Adata.Python }

// GetAdataScript 返回脚本路径
func GetAdataScript() string { return _map.Adata.Script }

// GetAdataArgs 返回脚本参数
func GetAdataArgs() []string { return _map.Adata.Args }

// GetSecurityConfig 返回安全配置副本
func GetSecurityConfig() SecurityConfig {
	return _map.Security
}

func ResolveAllowOrigin(origin string) (string, bool) {
	origin = strings.TrimSpace(origin)
	allowed := _map.Security.AllowedOrigins
	if len(allowed) == 0 {
		return "", false
	}
	for _, candidate := range allowed {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return "*", true
		}
		if origin != "" && strings.EqualFold(candidate, origin) {
			return candidate, true
		}
	}
	return "", false
}

func ShouldAllowCredentials() bool {
	if hasWildcardOrigin(_map.Security.AllowedOrigins) {
		return false
	}
	return _map.Security.AllowCredentials
}

func ShouldUseSecureCookies() bool {
	if _map.Security.CookieSecure != nil {
		return *_map.Security.CookieSecure
	}
	return !_map.IsDev
}

func GetCookieSameSite() http.SameSite {
	switch strings.ToLower(strings.TrimSpace(_map.Security.CookieSameSite)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func GetCookieDomain(fallbackHost string) string {
	domain := strings.TrimSpace(_map.Security.CookieDomain)
	if domain != "" {
		return domain
	}
	return sanitizeHost(fallbackHost)
}

func ShouldExposeMetrics() bool {
	return _map.Security.EnableMetrics
}

func hasWildcardOrigin(origins []string) bool {
	for _, o := range origins {
		if strings.TrimSpace(o) == "*" {
			return true
		}
	}
	return false
}

func sanitizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if idx := strings.Index(host, ":"); idx > -1 {
		host = host[:idx]
	}
	if strings.ContainsAny(host, "/\\") {
		return ""
	}
	if strings.EqualFold(host, "localhost") {
		return ""
	}
	return host
}
