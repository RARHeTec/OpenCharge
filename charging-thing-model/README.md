# charging-thing-model

**核心契约层** — 网关与业务之间唯一的沟通语言。任何硬件协议差异都必须在此层被翻译成标准事件，禁止把协议字段透传给业务模块。

## 事件清单

| 方向 | 事件 | 说明 |
|---|---|---|
| 上行 | `BootNotification` | 设备上线注册 |
| 上行 | `Heartbeat` | 心跳保活 |
| 上行 | `StatusNotification` | 枪头状态变更 |
| 上行 | `MeterValues` | 电表遥测（走快通道） |
| 上行 | `StopTransaction` | 停机账单，含 `Offline` 标记 |
| 上行 | `DiagnosticsResponse` | 诊断/日志抓取回执 |
| 下行 | `RemoteStartTransaction` | 远程启动 |
| 下行 | `RemoteStopTransaction` | 远程停止 |
| 下行 | `SetChargingProfile` | 功率调节 |
| 下行 | `ReserveNow` / `CancelReservation` | 预约 |
| 下行 | `TriggerMessage` | 远程诊断 / OTA |

## 关键字段

- `TraceId`：全链路追踪，网关入口生成
- `CommandSeq`：严格递增，防重放
- `TransactionId`：全局唯一，幂等控制
- `TariffVersionId`：电价快照版本，离线补单按历史价回溯
- `Offline`：桩端离线补传标记
