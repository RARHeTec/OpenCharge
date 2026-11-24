# charging-iot-gateway

独立部署的高并发物联网网关。职责：维持桩端长连接、编解码物理协议、翻译为统一物模型、分流到 Kafka 快慢通道。

## 子模块

- `protocol-spi`：协议适配器标准接口 + registry
- `adapter-yunkuaichong`：云快充 1.5/1.6 二进制帧
- `adapter-ocpp16`：OCPP 1.6J WebSocket/JSON
- `device-simulator`：虚拟桩，压测与联调

## 硬性规范

1. 收到字节流第一行即生成 TraceId，打印原始 Hex 报文（冷存储 7 天）
2. MeterValues → 高吞吐 Topic；状态变更 → 高优先级 Topic
3. 状态 Topic 以 `connectorId` 为分区键，保证同枪事件有序
4. CommandSeq 严格递增 + 时间戳窗口 <30s，防重放
5. 优雅下线：停止接收新连接 → 下发重连指令 → 限期断开（不等订单自然结束）
