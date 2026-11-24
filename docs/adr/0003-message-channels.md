# ADR-0003: 遥测与状态消息快慢通道隔离

- 状态：Accepted
- 日期：2026-11-24

## 背景

MeterValues 每秒上报，一旦积压会阻塞插枪/启动/停机结算等关键事件。

## 决策

两个独立 Topic：遥测高吞吐 Topic（批量写 TDengine），状态高优先级 Topic（同步驱动订单状态机）。状态 Topic 以 `{deviceId}:{gunNo}` 为分区键保证同枪有序。

## 后果

- 优点：结算不受遥测影响
- 代价： Gateway 需按事件类型路由
