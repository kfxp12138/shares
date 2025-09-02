# Shares 数据来源与调入细节

本页详细说明 Shares 项目各类数据的来源（外部接口）、解析与字段映射、调度触发点、落库策略（唯一键/更新策略）及可视化消费路径。

## 1. 总览

- 行业/板块数据（东财 push2/push2his）
  - 日净流入聚合（当日）：push2 `clist/get`（f62）
  - 历史日线：push2his `stock/kline/get`（k 线）
- 个股数据
  - 实时/分时/基本快照：腾讯 `qt.gtimg.cn`；分时/日线明细：腾讯 `web.ifzq.gtimg.cn`
  - 主力净流入（当日/历史）：东财 `ulist.np/get`、`fflow/daykline/get`
- 调度触发
  - 交易时段：`OnDealDay`/`OnDeal` 定时抓取与实时分析
  - 盘后：`DayAfter` 写入日线、计算 MA、汇总行业净流入
  - 工具模式（tools_type>0）：按不同工具启动独立采集/分析循环

## 2. 行业/板块

### 2.1 当日行业净流入（聚合）

- 源接口：
  - `https://push2.eastmoney.com/api/qt/clist/get?pn=1&pz=1000&np=1&fields=f12,f14,f62&fid=f62&fs=m:90`
  - 说明：`fs=m:90` 表示行业/概念板块列表；`fid=f62` 以主力净流入排序；分页 `pn`、每页 `pz`
- 字段映射（部分）：
  - `f12`: 行业/板块代码（如 BK0859）
  - `f14`: 行业/板块名称
  - `f62`: 主力净流入（原始单位，代码中统一换算为“万元”）
- 解析与落库：
  - 唯一键：`(hy_code, day0)`；当日 `day0 = tools.GetUtcDay0(time.Now())`
  - 新记录：`Create` 插入；已存在：按 `id` 执行 `Updates`
  - 换算：`zljlr = f62 * 0.0001`（单位转换为万元）
- 关键代码：
  - `shares/shares/internal/service/analy/hy.go:179`
  - `shares/shares/internal/service/event/event.go:293`
- 目标表：`hy_daily_tbl`
  - 主键：`id`（自增）；唯一键：`hy_code, day0`

### 2.2 行业历史日线（价格/量/换手）

- 源接口：
  - `https://46.push2his.eastmoney.com/api/qt/stock/kline/get?secid=90.<HY_CODE>&fields1=f1,f2,f3&fields2=f51,f52,f53,f54,f55,f56,f57,f59,f61&klt=101&fqt=1&end=20500101&lmt=<N>`
  - 说明：`secid=90.<HY_CODE>` 表示行业板块；`klt=101` 日线；`lmt` 拉取条数
- 字段解析（按代码实际使用）：
  - 返回 `klines` 每条形如：`yyyy-MM-dd,open,close,high,low,volume,turnover,percent,...`
  - 映射：
    - `day0`: 由日期字符串转时间戳
    - `price`: `close`
    - `percent`: `percent`
    - `volume`: `volume`
    - `turnover`: `turnover`
    - `turnover_rate`: 第 9 项（percent 后一位，见代码）
    - `max/min/open/close`: 对应高/低/开/收
- 计算与更新：
  - 对每个 `(hy_code, day0)` 记录，回查近 `N` 日均值，计算 `ma5/ma10/ma20/ma60` 并 `Update`
- 关键代码：
  - 抓取与落库：`shares/shares/internal/service/analy/hy.go:82`
  - 计算 MA：`shares/shares/internal/service/analy/hy.go:124`
- 目标表：`hy_daily_tbl`

## 3. 个股

### 3.1 搜索与基本快照（腾讯）

- 快速检索（简表）：
  - `http://qt.gtimg.cn/q=s_<code1>,s_<code2>,...`
  - 返回形如 `v_s_sh600000="<~分隔>";` 的多段文本，需 GBK→UTF-8 转码
  - 映射要点：`ext`（市场）、`name`、`simpleCode`、`price`、`percent` 等
  - 代码：`shares/shares/internal/service/event/search.go:27`
- 全量快照（明细）：
  - `http://qt.gtimg.cn/q=<code1>,<code2>,...`
  - 解析 `~` 分隔的字段数组，提取 `open/close/percent/volume/turnover/turnoverRate/pe/pb/max/min/总市值/流通市值` 等
  - 代码：`shares/shares/internal/service/event/search.go:58`

### 3.2 分时与日线（腾讯）

- 分时：
  - `https://web.ifzq.gtimg.cn/appstock/app/minute/query?code=<code>`
  - 解析分钟数据 `"HH:MM price vol ..."`，计算累计成交量与均价 `ave`
  - 代码：`shares/shares/internal/service/event/event.go:400`
- 前复权日线：
  - `https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?param=<code>,day,,,320,qfq`
  - JSON 结构中 `day` 列表：`[date, open, close, high, low, vol]`
  - 代码：`shares/shares/internal/service/event/event.go:520`

### 3.3 主力净流入（东财）

- 当日：
  - `https://push2.eastmoney.com/api/qt/ulist.np/get?secids=<mkt>.<code>&fields=f12,f13,f14,f62`
  - 市场：`sh -> 1`，`sz -> 0`，拼接 `secids=<mkt>.<simpleCode>`
  - 换算：`zljlr = f62 * 0.0001`（万元）
  - 代码：`shares/shares/internal/service/event/search.go:112`
- 历史：
  - `https://push2his.eastmoney.com/api/qt/stock/fflow/daykline/get?fields1=f1&fields2=f51,f52&secid=<mkt>.<code>`
  - 解析 `yyyy-MM-dd,net_inflow`，换算同上
  - 代码：`shares/shares/internal/service/event/search.go:84`

## 4. 调度与数据写入

- 交易时段（心跳）：`OnDealDay`/`OnDeal`
  - 扫描盯盘代码 → 腾讯 `Searchs` 拉取快照 → 入本地缓存（2h TTL）并触发规则分析（涨跌、阈值、MA 穿越等）
  - 代码：`shares/shares/internal/service/event/event.go:40, 78`
- 盘后：`DayAfter`
  - 扫描所有股票 → `SearchDetails` 拉取全量 → 更新 `shares_info_tbl`/写入 `shares_daily_tbl` → `SetDailyMa` 计算 MA
  - 同步 `zljlr_daily_tbl` 并执行 `countHYZLJLR(day0)` 更新当日行业净流入（`hy_daily_tbl`）
  - 代码：`shares/shares/internal/service/event/event.go:102, 120, 280, 560`
- 工具模式：`shares/shares/internal/service/analy/init.go:15`
  - `tools_type=4` 启动本地放量监听：周期调用 `GetMinute`、实时对比三分钟成交量窗口并推送

## 5. 表与唯一键

- `hy_daily_tbl`（行业/板块日线）：
  - 唯一键：`hy_code, day0`；主键自增 `id`
  - 字段：`price, percent, volume, turnover, turnover_rate, zljlr, ma5/10/20/60, open/close/max/min, pe/peg/pb ...`
- `shares_daily_tbl`（个股日线）：
  - 唯一键：`code, day0`；字段同上并含 `macd/dif/dea/vol`
- `zljlr_daily_tbl`（个股净流入日表）
- `shares_info_tbl`（个股信息与当日快照）

## 6. 写入策略与异常处理

- Upsert 策略
  - 以唯一键先查（`FetchUniqueIndexBy...`），无则 `Create`，有则按 `id` 走 `Updates/Save`
  - 行业净流入使用 `Create`/`Updates` 分离，避免携带历史 `id` 造成主键冲突
- 单位换算
  - 东财净流入字段 `f62`（及日净流入）统一乘 `0.0001` 记为“万元”
- 字符集
  - 腾讯 `qt` 与 `ifzq` 接口返回常为 GBK，需要转换为 UTF-8 再解析
- 节流与时间窗
  - 交易时段排除了 11:30~13:00 午间，盘后 `DayAfter` 再聚合
- 缓存
  - 盘中快照入 `mycache`，TTL 2 小时，降低接口压力

## 7. 可视化消费

- 静态页面：`/shares/echarts/echarts.html`
  - 分时：`/shares/api/v1/shares.minute`
  - 日线：`/shares/api/v1/shares.dayliy`
  - 示例：`http://localhost:8082/shares/echarts/echarts.html?tag=daily&code=sh600000`

## 8. 快速排错建议

- 行业净流入异常：检查 `hy.go` 是否写入 `zljlr`（非 `price`），并确认唯一键 `(hy_code, day0)` 正确
- 404 可视化：确认 `main.go` 已挂载静态目录 `/shares/echarts`、`/shares/docs`
- 端口占用/权限：开发建议使用 `8082`；生产使用 `82` 需 root 权限或以服务方式运行

## 9. 概念映射（代码 ↔ 概念/板块）

- 刷新入口：`POST /shares/api/v1/analy.refresh_concepts`
  - 支持两种来源：
    - 东财：触发 `initHY()` + `initHYCode()`，重建板块列表并回填 `shares_info_tbl.hy_name` + `concept_map_tbl`
    - adata：`POST /analy.refresh_concepts?source=adata`，Body 传入概念 JSON（详见 VISUAL_API），同样落表（概念归一化支持）
- 查询接口：
  - `GET/POST /shares/api/v1/analy.concepts_by_code?code=sh600000`
  - `GET /shares/api/v1/analy.search_concepts?q=机器人`
- 查询融合：
  - `Shares.Search` 已在返回体中的 `info.hy` 携带概念字符串（来自 `shares_info_tbl.hy_name`）
  - `shares.search_plus` 新增 `concepts: []string` 数组，便于联动
  - `shares.search_plus_detail` 返回 `concepts: [{id,name,hyCode}]` 明细，方便做概念跳转/过滤
  - 结构化表：
    - `concept_master_tbl(id,hy_code,name,created_at)`
    - `concept_alias_tbl(id,alias,name,created_at)`（用于 adata/东财 名称归一化）
    - `concept_map_tbl(id,code,hy_code,concept_id,name,created_at)`（唯一键：code,name）
