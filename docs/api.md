# API 与事件约定

## HTTP

- 网关健康：`GET /healthz`
- 核心服务健康：`GET /healthz`
- 租户白标配置：`GET /api/tenant/config?appId=...` 或 `?domain=...`

所有请求头携带 `X-Trace-Id`（缺失则网关注入）。响应统一为 `{code, message, data, traceId}`。

## Kafka Topic

| Topic | 用途 | 分区键 |
|---|---|---|
| `charging.telemetry.metervalues.v1` | MeterValues 批量遥测 | deviceId |
| `charging.state.events.v1` | 插枪/启动/停机/结算 | `{deviceId}:{gunNo}` |
| `charging.commands.v1` | 下行指令 | deviceId |

**快慢隔离**：遥测积压不得阻塞结算。遥测 batch 写入 TDengine，状态事件同步驱动订单状态机。

## 金额表示

全链路 `int64`，1 单位 = 0.0001 元（scale=4）。JSON 序列化为字符串（如 `"0.6234"`）避免 JS 精度丢失。仅在**边界层**（API 出参、结算落库、对账单）格式化到 2 位小数，half-up 舍入。

## 枪头锁

启动前必须持有 `lock:connector:{deviceId}:{gunNo}`，TTL 建议 30s，token 用 TraceId，释放走 Lua CAS 防止误删他人锁。
