# Shares 技术文档（后端与服务概览）

本文件为 `shares` 服务的技术说明，聚焦整体架构、关键目录、数据流、主要接口、运行部署与二次开发。

## 一、项目概述

- 定位：A 股量化交易与盯盘助手，包含日常数据采集、实时行情监控、技术/消息分析、微信提醒、组织/分组管理与可视化。
- 技术栈：Go（Gin + ginrpc + goplugins/micro）、GORM（自动模型）、MySQL、Redis、Prometheus、Swagger；配套前端（uniapp 小程序、Element+Webpack）与 Python 技术指标脚本。
- 运行形态：HTTP 服务（开发端口 8082，生产可配 82，API 前缀 `/shares/api/v1`），可选微服务/插件化运行。

## 二、目录结构（关键）

- 入口与配置
  - `shares/shares/main.go`
  - `shares/shares/conf/config.yml`
- 路由与注册
  - `shares/shares/internal/routers/api_root.go`
  - `shares/shares/internal/routers/gen_router.go`
- 配置中心
  - `shares/shares/internal/config/*.go`
- 数据访问与模型
  - `shares/shares/internal/core/dao.go`
  - `shares/shares/internal/model/*.go`（gormt 自动生成）
- 业务服务
  - 股票服务：`shares/shares/internal/service/shares/*.go`
  - 分析服务：`shares/shares/internal/service/analy/*.go`
  - 事件/调度/采集：`shares/shares/internal/service/event/*.go`
  - 微信服务：`shares/shares/internal/service/weixin/*.go`
  - NLP/AI（可选）：`shares/shares/internal/service/nlp/*.go`
- 文档/可视化
  - API 文档（生成）：`shares/shares/docs/markdown/*.md`
  - Swagger：`shares/shares/docs/swagger/swagger.json`
  - ECharts 页面：`shares/shares/echarts/echarts.html`
- 原始 proto：`shares/apidoc/proto/*`
- 数据库脚本：`shares/mysql/*.sql`
- 前端（示例）
  - UniApp 小程序：`shares/shares/uniapp/*`
  - Web Demo（Element+Webpack）：`shares/element/webpack/*`

## 三、架构与启动流程

1. 入口 `main.go` 读取配置、初始化 Gin、注册路由与服务对象，并通过 `plugin.RunHTTP` 启动 HTTP。
2. `api_root.go` 内注册服务对象（Weixin、Shares、Analy），绑定到路由组 `/shares/api/v1`，并开放健康检查 `/health` 与 Prometheus `/metrics`。
3. `ginrpc` 自动将对象方法映射为 REST 路由（别名在 `gen_router.go` 维护）。
4. 数据层通过 `core/dao.go` 创建 MySQL 读写 ORM 句柄；模型由 gormt 生成在 `internal/model`。

## 四、主要服务与接口（概览）

参考生成文档：
- 股票：`shares/shares/docs/markdown/Shares.md`
- 分析：`shares/shares/docs/markdown/Analy.md`
- 微信：`shares/shares/docs/markdown/Weixin.md`

核心能力概览：
- Shares
  - 搜索与详情：`Shares.Search`（支持中文名/代码，首查入库、补全、日线/图像链接等）、`Shares.Gets`（批量精确）
  - 图形数据：`Shares.Minute`（分时）/`Shares.Dayliy`（日 K，含 MA）
  - 盯盘与分组：`AddMyCode`/`GetMyCode`/`DeleteMyCode`，`GetGroup`/`GetMyGroup`/`UpsetGroupCode`/`AddGroup`
  - 消息：`GetMsg`（最近 10 条）/`HaveNewMsg`（日 Badge）
- Analy
  - `Analy.AnalyCode` 聚合技术/消息策略（MACD、北上、龙虎榜、主力净流入、成交量、十字星、线性预测、情绪关键词等）
- Weixin
  - `Oauth`/`ReLogin` 授权登录并设置 Cookie（`user_token`/`session_token`）
  - `GetUserInfo`/`UpsetUserInfo` 用户资料读取与更新（涨绿跌红、仅 20 日、手机号等）
  - `GetQrcode` 生成页面二维码

## 五、数据采集与调度（事件子系统）

- 行情与数据源：腾讯行情（qt.gtimg.cn）、东方财富（push2/push2his）。
- 运行管线：
  - 盘中（OnDealDay/OnDeal）：按周期拉取盯盘股票实时行情，写入缓存（2h TTL），并触发规则（价格突破、百分比、MA 上下穿、快速涨跌）。
  - 收盘后（DayAfter）：批量刷新 `shares_info_tbl` 快照、写入 `shares_daily_tbl`、计算 MA5/10/20、行业主力净流入等。
  - 固定时点：每日 8:00 北上数据、20:00 龙虎榜数据（maBS/maLhb）。
- 实时报警：
  - 快速涨跌：三分钟窗口，涨幅≥3% 或跌幅≥4%（午后权重加倍）；
  - 价格/百分比阈值：Up/Down/UpPercent/DownPercent；
  - MA 实时：跌破/站上 5/10/20 日线（20 日可放宽条件）。
  - 消息：模板消息写入 `msg_tbl` 并通过微信发送（去重记录在 `msg_rapidly_tbl`）。

## 六、数据模型（核心表）

- `shares_info_tbl`：个股基础与当日快照（代码、名称、拼音、价格、涨跌、行业、PEG、总/流通市值等）
- `shares_daily_tbl`：日线数据（day0、price、percent、volume、turnover、turnover_rate、ma5/10/20、zljlr、open/close、macd/dif/dea 等）
- `shares_watch_tbl`：用户盯盘条件（价格/百分比/KDJ/20 日线/公开等）
- `msg_tbl`、`msg_rapidly_tbl`：推送消息与去重标记
- `wx_userinfo`：用户信息（openid、昵称、分组、容量、偏好等）
- `group_tbl`、`group_list_tbl`：分组与成员关系
- 行业与榜单：`hy_daily_tbl`、`lhb_daily_tbl`、`zljlr_daily_tbl`

模型文件位于 `shares/shares/internal/model/*.go`。

## 七、配置说明

- 文件：`shares/shares/conf/config.yml`
- 关键项：
  - `base.is_dev`：开发模式开关
  - `tools_type`：0 启用定时任务；>0 表示工具模式（跳过定时调度）
  - `db_info` / `redis_info` / `etcd_info`
  - `wx_info`：微信相关（AppID/AppSecret/APIKey/MchID/NotifyURL/ShearURL）
  - `port`：默认 82；`file_host`：二维码等外链前缀
  - `max_capacity`：用户默认容量；`def_group`：默认分组；`ext`：`[sh,sz,hk]`

## 八、运行与构建

1) 准备
- 安装 Go、MySQL、Redis；（可选）安装 protoc 以生成 pb。
- 修改 `shares/shares/conf/config.yml` 对接本地环境。

2) 初始化数据库
```
mysql -h127.0.0.1 -uroot -p123456 -e "CREATE DATABASE IF NOT EXISTS caoguo_dev DEFAULT CHARSET utf8mb4;"
mysql -h127.0.0.1 -uroot -p123456 caoguo_dev < shares/mysql/shares_tmp_db.sql
mysql -h127.0.0.1 -uroot -p123456 caoguo_dev < shares/mysql/shares_tmp_db_views.sql
```

3) 本地启动
```
cd shares/shares
make run
# 或
go build -o shares *.go && ./shares debug
```

4) 生产部署（服务方式）
```
sudo ./shares install && sudo ./shares start
# 停止或前台运行
sudo ./shares stop
sudo ./shares run
```

## 九、可视化与前端

- ECharts 页面：`shares/shares/echarts/echarts.html`
  - 参数：`code`（如 sh600000）、`rg`（true 涨红/false 涨绿）、`only20`（仅显示 20 日线）、`tag`（`min`/`daily`）。
  - 示例：`http://localhost:8082/shares/echarts/echarts.html?rg=true&only20=false&tag=daily&code=sh600000`
- 选股板块榜单：`shares/shares/echarts/board.html`（依赖接口 `/shares/api/v1/analy.pick_board`）
  - 示例：`http://localhost:8082/shares/echarts/board.html`
- 小程序：`shares/shares/uniapp/*`（在 `utils/server/*.js` 中配置服务地址）
- Web Demo：`shares/element/webpack/*`

## 十、监控与健康

- 健康检查：`GET /shares/api/v1/health`（在路由组下为 `/health`）
- Prometheus 指标：`GET /shares/api/v1/metrics`（在路由组下为 `/metrics`）

## 十一、安全与鉴权

- Cookie：`user_token`（openid）与 `session_token`（会话），在 `Weixin.Oauth`/`ReLogin` 设置。
- 用户态缓存：`internal/api/user.go` 使用 `mycache` 缓存用户信息以减少数据库查询。
- 支付/证书：证书位于 `conf/cert/*`，生产环境建议通过安全配置中心挂载。

## 十二、扩展与二次开发

- 新增接口流程：
  1. 在 `shares/apidoc/proto/shares/*.proto` 定义请求/响应与服务 RPC；
  2. `cd shares/shares && make gen` 生成 pb 与 ginrpc 绑定；
  3. 在对应服务对象实现方法（如 `internal/service/shares/*.go`）；
  4. 文档与路由别名自动更新（`docs/markdown/*`、`internal/routers/gen_router.go`）。
- 定时任务：根据 `tools_type` 控制是否启用；可在 `internal/service/event/event.go` 扩展 `OnDeal`/`DayAfter`。
- 策略扩展：在 `internal/service/analy/*.go` 新增策略并汇总到 `AnalyCode` 返回。

## 十三、关键文件速查

- 入口与路由：`shares/shares/main.go`、`shares/shares/internal/routers/api_root.go`
- 股票服务：`shares/shares/internal/service/shares/shares.go`
- 分析服务：`shares/shares/internal/service/analy/analy.go`
- 事件调度：`shares/shares/internal/service/event/event.go`
- 行情抓取：`shares/shares/internal/service/event/search.go`
- 微信服务：`shares/shares/internal/service/weixin/weixin.go`
- 配置中心：`shares/shares/conf/config.yml`、`shares/shares/internal/config/*.go`
- ORM 模型：`shares/shares/internal/model/*.go`
- API 文档：`shares/shares/docs/markdown/*.md`、`shares/shares/docs/swagger/swagger.json`

## 十四、快速开始（从零到可见数据）

1) 导入 SQL 并启动服务（参考“运行与构建”）。
2) 打开浏览器访问：
   - Swagger JSON：`http://localhost:8082/shares/docs/swagger/swagger.json`
   - ECharts 日线示例：`http://localhost:8082/shares/echarts/echarts.html?tag=daily&code=sh600000`
3) 调用接口：
   - `POST /shares/api/v1/shares.search`，body：`{"code":"600000"}`（服务自动补全 `sh600000`）。
   - 返回中 `img` 字段即 K 线可视化地址。

## 十五、常用命令

```
cd shares/shares
make orm           # 生成 GORM 模型（需 tools/gormt 可执行）
make gen           # 生成 proto 绑定与文档
make run           # 本地构建并运行
```

—— 如需补充 API 清单/时序图/部署图，请在此文件继续扩展。

## 十六、数据来源与调入细节

- 概览与字段映射、调度触发、落库策略等请见：`shares/shares/docs/DATAFLOW.md`

## 十七、可视化 API 清单

- 可视化页面与数据 API、返回 `img` 链接的业务接口见：`shares/shares/docs/VISUAL_API.md`
