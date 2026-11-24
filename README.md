# OpenCharge

> 开源、可商用、私有化部署的新能源充电桩运营管理平台（SaaS / PaaS）

OpenCharge 是一个从零构建的充电平台技术底座，面向平台自营、运营商多租户加盟与第三方互联互通场景。项目采用 Go 后端 + Vue 3 / uni-app 前端，核心设计遵循三条原则：**协议与业务解耦**、**账务精度零误差**、**多租户数据强隔离**。

## 目录结构

```
OpenCharge/
├── charging-common/            # 公共基础包：金额定点数、多租户隔离、分布式锁、TraceId、脱敏加密
├── charging-thing-model/       # 核心契约：统一物模型与上下行标准事件 DTO
├── charging-iot-gateway/       # 独立部署的物联网网关（协议 SPI + 适配器 + 桩模拟器）
├── charging-core-service/      # 核心业务域（模块化单体，可按需拆分）
├── charging-deploy/            # 私有化交付：Docker Compose / Helm / 初始化 SQL
├── charging-frontends/         # 前端同仓分目录：admin + uni-app + shared
└── docs/                       # 架构文档、ADR 决策记录、API 文档
```

## 核心能力

- 统一物模型：网关与业务之间唯一通信语言，屏蔽云快充 / OCPP 等物理协议差异
- 精确计费：全链路 `int64` 定点数（scale=4，1 单位 = 0.0001 元），边界层统一格式化与舍入
- 枪头级并发控制：启动充电强制加 `lock:connector:{deviceId}:{gunNo}` 分布式锁
- 消息快慢通道：遥测走高吞吐 Topic，状态变更走高优先级 Topic
- 多租户隔离：GORM Scope 自动注入 `tenant_id`，缺失上下文时 fail-closed
- 弱网补偿：离线账单补传 + 电价版本快照回溯计费
- 全链路追踪：网关入口生成 TraceId，原始 Hex 报文日志可追溯

## 快速开始

```bash
go work sync
go build ./...
```

详见 [docs/architecture.md](docs/architecture.md) 与 [charging-deploy/README.md](charging-deploy/README.md)。

## 技术选型

| 层 | 选型 |
|---|---|
| 后端 | Go 1.22+ |
| 前端 | Vue 3 + Vite（管理后台）、uni-app（小程序） |
| 网关 | Go net / goroutine，EMQX 作为 MQTT 中间件 |
| 存储 | PostgreSQL / MySQL、TDengine、Redis、Kafka |
| 部署 | Docker Compose（中小规模）、Kubernetes + Helm（大规模） |

## 许可证

Apache License 2.0 — 允许商用、修改、分发，需保留署名与 NOTICE。详见 [LICENSE](LICENSE) 与 [NOTICE](NOTICE)。
