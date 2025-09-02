# Shares 可视化 API 清单（分时/日K/图表链接）

本文整理项目中可直接用于可视化（ECharts/Grafana/Swagger）的接口、参数与返回结构，涵盖静态页面与数据 API，并给出可嵌入示例。

> 开发默认端口：`8082`（见 `conf/config.yml: port`）。生产可改回 82（需 root）。

## 1) 静态页面（ECharts）

- 页面：`/shares/echarts/echarts.html`
  - 说明：内置 ECharts 渲染逻辑，通过 AJAX 调用后台数据 API（见下文 2.1/2.2）。
  - 查询参数：
    - `code`：股票代码（如 `sh600000`）
    - `tag`：`min`（分时）或 `daily`（日K）
    - `rg`：`true|false`，涨红 or 涨绿
    - `only20`：`true|false`，日K仅显示 20 日线
  - 示例：
    - 日K：`http://localhost:8082/shares/echarts/echarts.html?tag=daily&code=sh600000`
    - 分时：`http://localhost:8082/shares/echarts/echarts.html?tag=min&code=sh600000`
  - 代码参考：
    - 静态挂载：`shares/shares/main.go:49`
    - 页面调用：`shares/shares/echarts/echarts.html:60`（日K）、`:94`（分时）

- 页面：`/shares/echarts/board.html`
  - 说明：选股板块（热板）榜单，展示板块涨幅/换手/净流入及领涨股，数据源为 3.1 节的接口。
  - 示例：`http://localhost:8082/shares/echarts/board.html`
  - 代码参考：`shares/shares/echarts/board.html`

- 页面：`/shares/echarts/myboard.html`
  - 说明：自选股榜单，展示代码、名称、涨幅、现价、几板、首封时间、概念、现量、涨速、换手。
  - 示例：`http://localhost:8082/shares/echarts/myboard.html`（需要已登录）
  - 代码参考：`shares/shares/echarts/myboard.html`

- 页面：`/shares/echarts/watchlist.html`
  - 说明：前端维护的“自选股分组”，本地存储（localStorage），无需 Cookie/登录；通过接口聚合展示。
  - 示例：`http://localhost:8082/shares/echarts/watchlist.html`
  - 代码参考：`shares/shares/echarts/watchlist.html`

## 2) 数据 API（页面数据源）

- POST `/shares/api/v1/shares.dayliy`（日K数据）
  - 入参：`{"code":"sh600000"}`
  - 返回：二维数组，每项为 `[date, open, close, high, low, vol]`
  - 示例：
    ```json
    [["2025-08-20",9.99,10.10,10.50,9.80,123456]]
    ```
  - 代码参考：
    - 接口：`shares/shares/internal/service/shares/shares.go:681`
    - 构建：`shares/shares/internal/service/event/event.go:520`

- POST `/shares/api/v1/shares.minute`（分时数据）
  - 入参：`{"code":"sh600000"}`
  - 返回：
    ```json
    {
      "yestclose": 9.85,
      "ref": true,
      "data": [["09:30", 10.01, 9.95, 1200], ["09:31", 10.02, 9.96, 800]]
    }
    ```
    - `data` 子项：`[time, price, ave, vol]`
    - `ref`：是否建议继续轮询（盘中为 true；收盘/周末为 false）
  - 代码参考：
    - 接口：`shares/shares/internal/service/shares/shares.go:648`
    - 构建：`shares/shares/internal/service/event/event.go:400`

## 3) 返回“可视化链接”的业务 API

- POST `/shares/api/v1/shares.search`（搜索并返回图表链接）
  - 入参：`{"code":"600000","tag":"daily"}`（服务会自动补全 `sh/sz`）
  - 返回：
    ```json
    {
      "info": {
        "code": "sh600000",
        "name": "浦发银行",
        "price": 10.01,
        "percent": 1.23,
        "img": "/webshares/echarts/echarts.html?rg=true&only20=false&tag=daily&code=sh600000"
      }
    }
    ```
    - `img` 字段为可直接打开的 ECharts 链接。
    - 本地直连可将前缀 `/webshares` 调整为 `/shares/echarts`。
  - 文档参考：`shares/shares/docs/markdown/Shares.md:388`
  - 代码参考：`shares/shares/internal/service/shares/shares.go:269`

- POST `/shares/api/v1/shares.get_group`（分组 + 成员项含图表链接）
  - 返回的成员对象里含 `img` 字段（同上，可直接渲染为链接/按钮）。
  - 文档参考：`shares/shares/docs/markdown/Shares.md:520`

- GET/POST `/shares/api/v1/analy.pick_board`（选股板块/热板榜单）
  - 入参（query 或 JSON）：`{ "limit": 30, "leaders": 1 }`
    - `limit`：返回板块数，默认 30
    - `leaders`：每个板块返回的领涨股数量，默认 1
  - 返回：`{ list: [ { hyCode, hyName, percent, turnoverRate, num, up, zljlr, leaders: [ { code, name, price, percent } ] } ] }`
  - 说明：
    - 板块来源：`hy_up_tbl` 当日记录
    - 净流入：`hy_daily_tbl` 当天 `zljlr`（万元）
    - 领涨股：通过 `shares_info_tbl.hy_name like %板块名%` 获取候选，再实时行情排序
  - 代码参考：
    - 路由注册：`shares/shares/internal/routers/api_root.go`
    - 实现：`shares/shares/internal/service/analy/board.go`

- GET/POST `/shares/api/v1/analy.my_board`（自选股榜单）
  - 入参（query 或 JSON）：`{ "limit": 100 }`
  - 返回：`{ list: [ { code,name,percent,price,boards,firstSeal,concepts,curVol,speed,turnoverRt } ] }`
  - 说明：
    - 代码集合来自当前登录用户的 `shares_watch_tbl`
    - 涨幅/现价：实时快照（腾讯）
    - 换手率：实时明细（腾讯全量接口）
    - 现量/涨速/首封时间：分钟线估算（ifzq），涨停阈值按 10%/20%/5% 规则近似
    - 几板：以 `shares_daily_tbl` 连续涨停 + 当日实时近似
  - 代码参考：
    - 路由注册：`shares/shares/internal/routers/api_root.go`
    - 实现：`shares/shares/internal/service/analy/myboard.go`

- GET/POST `/shares/api/v1/analy.pick_codes`（自选代码榜单，无需登录）
  - 入参：`codes` 逗号分隔或 JSON 数组，`limit`（默认 100）
  - 返回：同上 `MyBoardResp` 结构，用于前端自选分组聚合
  - 代码参考：
    - 路由注册：`shares/shares/internal/routers/api_root.go`
    - 实现：`shares/shares/internal/service/analy/pick_codes.go`

- GET `/shares/api/v1/analy.quick_search`（简易搜索，无需登录）
  - 入参：`q`（名称关键字或代码前缀）
  - 返回：`{ list: [ { code,name,hyName } ] }`（最多 10 条）
  - 用途：前端 watchlist 名称快速补全为标准代码

- GET/POST `/shares/api/v1/analy.dev_login`（一键登录，开发用）
  - 入参：`openid`（可选，默认 `dev_openid`），`nick`（可选）
  - 效果：创建/更新 `wx_userinfo`，并设置 Cookie：`user_token`（openid）、`session_token`（sessionId）
  - 用途：在无微信的环境下快速获得登录态，以访问需要登录的接口/页面（如 myboard）

## 4) 监控与文档（可视化相关）

- GET `/shares/api/v1/metrics`（Prometheus 指标，可接入 Grafana）
  - 入口：`shares/shares/internal/routers/api_root.go:39`
- GET `/shares/api/v1/health`（健康检查）
- GET `/shares/docs/swagger/swagger.json`（Swagger JSON）
  - 静态挂载：`shares/shares/main.go:50`

## 5) 嵌入与调用示例

- 在任意网页嵌入图表（iframe）：
  ```html
  <iframe src="http://localhost:8082/shares/echarts/echarts.html?tag=daily&code=sh600000"
          style="width:100%;height:520px;border:0;"></iframe>
  ```
- 直接拉取数据（日K）：
  ```bash
  curl -X POST \
    http://localhost:8082/shares/api/v1/shares.dayliy \
    -H 'Content-Type: application/json' \
    -d '{"code":"sh600000"}'
  ```
- 直接拉取数据（分时）：
  ```bash
  curl -X POST \
    http://localhost:8082/shares/api/v1/shares.minute \
    -H 'Content-Type: application/json' \
    -d '{"code":"sh600000"}'
  ```

## 6) 常见问题

- 404 页面：确认服务已挂载静态目录（main.go 已注册 `/shares/echarts`、`/shares/docs`），并重启进程。
- 无法继续刷新：分时接口返回 `ref=false` 表示当前不在交易活跃时段，可停止轮询。
- 跨域：已启用 CORS，可直接在浏览器前端调用 API（见 `routers.Cors()`）。

## 7) 快速访问示例

日K图: http://localhost:8082/shares/echarts/echarts.html?tag=daily&code=sh600000
分时图: http://localhost:8082/shares/echarts/echarts.html?tag=min&code=sh600000
热板榜单: http://localhost:8082/shares/echarts/board.html
自选榜单(带一键登录): http://localhost:8082/shares/echarts/myboard.html
本地自选分组(多分组): http://localhost:8082/shares/echarts/watchlist.html
打开导入页面: http://localhost:8082/shares/echarts/import_concepts.html
一键登录（开发）

直接打开: http://localhost:8082/shares/api/v1/analy.dev_login?openid=dev_openid&nick=%E5%BC%80%E5%8F%91%E8%80%85
或在 myboard 页面点击“一键登录（开发）”
