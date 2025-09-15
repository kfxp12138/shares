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

- 页面：`/shares/echarts/limitup_calendar.html`
  - 说明：输入一只股票，聚合其“同概念”成分股，按交易日历显示每日达标数量；支持口径切换：大涨（≥阈值%）或涨停（10%/20%/ST 5%）。点击某一天可进入当日明细。
  - 参数：
    - `code`（如 `sh600000`）
    - `limit` 每个概念纳入的前 N 只（默认 60）
    - `days` 最近交易日个数（默认 22，约一个月）
    - `mode` 统计口径：`bigrise|limitup`（默认 `bigrise`）
    - `thType` 阈值类型：`rel|abs`（默认 `rel` 相对涨停）
    - `minRate` 相对阈值（0~1，默认 `0.7`，表示 ≥ 涨停幅度的 70%）
    - `minPct` 绝对阈值（单位 %，默认 7；当 `thType=abs` 时使用）
    - `concepts` 可多值或逗号分隔，仅统计选中概念
  - 示例：
    - 大涨（月视图，相对阈值 70%）：`http://localhost:8082/shares/echarts/limitup_calendar.html?code=sh600000&mode=bigrise&thType=rel&minRate=0.7&days=22`
    - 大涨（绝对阈值 8%）：`http://localhost:8082/shares/echarts/limitup_calendar.html?code=sh600000&mode=bigrise&thType=abs&minPct=8&days=22`
    - 指定概念：`http://localhost:8082/shares/echarts/limitup_calendar.html?code=sh600000&concepts=人工智能&concepts=CPO&mode=bigrise&thType=rel&minRate=0.7`
  - 数据源：后端聚合 `/analy.same_concept_limitup_calendar`（兼容上述参数）
  - 代码参考：`shares/shares/echarts/limitup_calendar.html`

- 页面：`/shares/echarts/limitup_day.html`
  - 说明：指定日期展示与主股票“同概念”的达标（大涨/涨停）股票列表；若为“今天”且口径为“涨停”，额外估算首封时间。包含“对比”区，基于 `/analy.compare_concepts` 与主股票概念做重叠分析。
  - 参数：
    - `code`（主股票，必填），`date`（YYYY-MM-DD，默认今天）
    - `mode`（`bigrise|limitup`，默认 `bigrise`）、`minPct`（仅在 `bigrise` 下生效，默认 7）
    - `concepts`（同上：筛选参与统计的概念）
  - 示例：`http://localhost:8082/shares/echarts/limitup_day.html?code=sh600000&date=2025-08-20&mode=bigrise&minPct=7`
  - 数据源：后端聚合 `/analy.same_concept_limitup_day`（兼容上述参数）
  - 代码参考：`shares/shares/echarts/limitup_day.html`

## 2.1) 新增后端聚合接口

- GET/POST `/shares/api/v1/analy.same_concept_limitup_calendar`
  - 入参：`code`、`days`（默认 22）、`perConcept`（默认 60）、`concepts`，以及 `mode`（`bigrise|limitup`）、`thType`、`minRate`、`minPct`
  - 返回：`{ code, concepts: [string], codes: int, items: [ { date, count, mainUp } ] }`
  - 说明：按主股票概念收集成分股，按近 N 个交易日统计达标数量；大涨模式支持两种阈值：
    - 相对阈值：≥ 各自涨停幅度的 `minRate`（例如主板 10%→7%，创业板/科创板 20%→14%，ST 5%→3.5%）
    - 绝对阈值：≥ `minPct%`
    涨停模式按 10%/20%/ST 5% 判断；同时返回当日主股票是否也达标（`mainUp`）。

- GET/POST `/shares/api/v1/analy.same_concept_limitup_day`
  - 入参：在原有基础上，支持 `mode`（`bigrise|limitup`）、`thType`、`minRate`、`minPct`。
  - 返回：`{ code, date, items: [ { code,name,percent,firstSeal } ] }`（大涨模式不返回 `firstSeal`）
  - 说明：返回指定日期所有“同概念达标”（大涨/涨停）的股票；当天且为涨停模式时估算分钟级“首封时间”。

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

## 7) 概念导入（定时 + 手动 + 立即）

- 定时（推荐）：在 `conf/config.yml` 配置 `adata.concepts_url`，服务将于每日 08:00 自动拉取并刷新概念映射。
- 手动：打开导入页面 `/shares/echarts/import_concepts.html` 粘贴 JSON 或提供 URL；或直接调用 `POST /shares/api/v1/analy.refresh_concepts?source=adata`。
- 立即：`GET/POST /shares/api/v1/analy.refresh_concepts_now` 按配置 URL 立即刷新。

## 8) 概念重叠分析（A vs B）

- 页面：`/shares/echarts/compare_concepts.html`
  - 说明：输入两组股票 A（参考）与 B（目标），按 B 与 A 概念重叠数量降序排序，并列出重叠概念。
  - 行为：页面并发调用后端接口获取概念与名称，无需登录。

- 后端接口：
  - POST `/shares/api/v1/analy.compare_concepts`
    - 入参（JSON 或表单）：`{ "codesA": ["sh600000"], "codesB": ["sz000001"], "onlyOverlap": true }`
      - 也支持字符串：`codesA="sh600000, sz000001"`（逗号/空白分隔）
    - 加权排序：按 `weighted`（重叠概念在 A 中频次求和）降序，其次 `overlap` 降序
    - 返回：
      ```json
      {
        "rows": [
          { "code": "sh600036", "name": "招商银行", "overlap": 3, "weighted": 7,
            "concepts": ["银行", "金融科技", "MSCI中国"],
            "conceptsDetail": [ {"name":"银行","count":4}, {"name":"金融科技","count":2}, {"name":"MSCI中国","count":1} ]
          }
        ],
        "conceptsA": [ { "name": "银行", "count": 4 }, ... ]
      }
      ```
  - POST `/shares/api/v1/analy.compare_concepts_export`
    - 入参同上，返回 CSV（`code,name,overlap,weighted,concepts_with_weight`），`Content-Disposition: attachment`。

## 9) 快速访问示例

日K图: http://localhost:8082/shares/echarts/echarts.html?tag=daily&code=sh600000
分时图: http://localhost:8082/shares/echarts/echarts.html?tag=min&code=sh600000
热板榜单: http://localhost:8082/shares/echarts/board.html
自选榜单(带一键登录): http://localhost:8082/shares/echarts/myboard.html
本地自选分组(多分组): http://localhost:8082/shares/echarts/watchlist.html
打开导入页面: http://localhost:8082/shares/echarts/import_concepts.html
一键登录（开发）

直接打开: http://localhost:8082/shares/api/v1/analy.dev_login?openid=dev_openid&nick=%E5%BC%80%E5%8F%91%E8%80%85

http://localhost:8082/shares/echarts/compare_concepts.html
或在 myboard 页面点击“一键登录（开发）”


http://localhost:8082/shares/echarts/limitup_calendar.html?code=sh600000&concepts=人工智能&concepts=CPO


口径

大涨:bigrise：按“涨幅达到某个阈值”统计。阈值可用两种方式设置（见下方“阈值”）。更灵活，适合跨板块（主板/创业板/科创板/ST）比较。
涨停:limitup：按“是否涨停”统计。涨停幅度依据标的所属规则自动判断：
主板等：10%
创业板/科创板：20%
ST：5%
适用范围：既影响日历中“当日达标数量”的计算，也影响明细页的入选标准；涨停口径在“今天”场景会额外估算首封时间。
阈值

相对阈值:thType=rel + minRate：以“各自涨停幅度×比例”作为达标标准（默认）。举例：minRate=0.7
主板(10%) → 10%×0.7=7%
创/科(20%) → 20%×0.7=14%
ST(5%) → 5%×0.7=3.5%
适合跨不同涨停制度的股票统一口径比较。
绝对阈值:thType=abs + minPct：以固定百分比作为达标标准（如 minPct=8 表示涨幅≥8%即达标）。
优先级与容差：
后端以 minRate>0 时优先采用“相对阈值”，否则使用“绝对阈值”。
计算时允许 0.2 个百分点容差，用于避免计算/数据源误差。
默认值（页面已内置，可在地址栏或控件修改）：
mode=bigrise（大涨）、thType=rel（相对阈值）、minRate=0.7、days=22（近一月交易日）
若切换为 mode=limitup（涨停），将按 10%/20%/5% 规则计算，忽略 minRate/minPct。
