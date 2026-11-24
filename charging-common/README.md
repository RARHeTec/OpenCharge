# charging-common

公共基础包。所有业务模块与网关都必须通过本包处理金额、租户上下文、分布式锁、TraceId 与脱敏。

| 包 | 职责 |
|---|---|
| `money` | int64 定点金额（scale=4），禁止浮点 |
| `tenant` | 租户上下文与 GORM Scope（fail-closed） |
| `lock` | 枪头级 Redis 分布式锁 |
| `trace` | TraceId 生成、Context / Kafka header 透传 |
| `crypto` | AES-256-GCM 加密与字段脱敏 |
| `response` | 统一 API 响应结构 |
