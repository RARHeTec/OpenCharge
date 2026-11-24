# OpenCharge 项目长期约定

## 项目定位
开源免费、可商用的新能源充电桩管理平台（SaaS/PaaS），基于用户提供的《私有充电平台（SaaS_PaaS）系统架构与从零开发规划文档.docx》。
目标组织：`RARHeTec`（GitHub 唯一可用组织），仓库名 `OpenCharge`，许可证建议 Apache-2.0（署名 + 可商用 + 开源）。

## 技术栈（2026-11-24 用户提出，讨论中）
- **后端 Go + 前端 Vue 3（uni-app C 端 + Vue3 管理后台）**。前端与文档原选型一致；后端由文档推荐的 Java 改为 Go。
- Go 带来的方案调整：IoT 网关 Netty→Go net/goroutine（EMQX 作为独立中间件保留）；Spring Cloud 微服务→模块化单体优先；Maven pom→go.work workspace。
- 三处需重新拍板：①金额类型（Go 无 BigDecimal 标准库，建议 int64 定点数 scale=4，即 1 单位=0.0001 元，可精确表示 4 位电价，结算整数除以 100 到分；优于 shopspring/decimal 与 3 位"毫"，因毫无法表示 4 位电价）；②多租户隔离（无 MyBatis-Plus 拦截器，改用 GORM Scope + 强制约定）；③SPI 插件（interface + registry 替代 Java SPI）。

## 用户确认的目录结构（charging-* 前缀，以此为准）
```
OpenCharge/
├── charging-common/            # 多租户拦截器、统一响应、分布式锁、加密脱敏
├── charging-thing-model/       # 核心契约：统一物模型、上下行标准事件 DTO
├── charging-iot-gateway/       # 独立部署：Go net/goroutine + EMQX
│   ├── protocol-spi/           # 协议适配器 interface + registry
│   ├── adapter-yunkuaichong/   # 云快充 1.5/1.6
│   ├── adapter-ocpp16/         # OCPP 1.6J
│   └── device-simulator/       # 虚拟桩压测联调
├── charging-core-service/      # 核心业务域（模块化单体，日后可拆）
│   ├── module-iam/             # OAuth2.1、多租户组织树、B2B2C授权
│   ├── module-device/          # 场站/桩/枪台账、远程运维
│   ├── module-order-billing/   # 生命周期状态机、48时段计费
│   ├── module-payment-clearing/# 钱包、分账路由、T+1对账
│   └── module-openapi/         # 第三方对接开放平台
└── charging-deploy/            # Docker Compose + K8s Helm + 初始化 SQL
```
补充建议（待确认）：`docs/`、`LICENSE/NOTICE`、`CONTRIBUTING.md`、根 `go.work`、`go.mod`、`charging-gateway/`（C端Web网关）。

## 必须死守的研发规范（用户明确要求）
1. **金额禁浮点**：电费/服务费/余额一律禁 float。Go 语境建议 int64 定点数 scale=4（单位 0.0001 元，覆盖 4 位电价），结算时整数除法到分；不用 float64，不引第三方 decimal。
2. **分布式锁到枪头**：扫码启动必须加 `lock:connector:{deviceId}:{gunNo}`，防重复下发启动指令。
3. **消息快慢通道**：MeterValues 遥测走高吞吐 Topic 批量写 TDengine；插枪/启动/停机状态变更走高优先级 Topic，防遥测积压阻塞结算。状态 Topic 以 connectorId 做分区键保序。
4. **断网续充/弱网补偿**：桩端离线凭白名单充电，恢复后补传 Offline Transaction；后端支持延迟补单 + 历史时段电价回溯（订单冗余 tariff_version_id 存电价快照）。
5. **TraceId 全链路**：网关接收字节流第一行即生成全局 TraceId，打印原始 Hex 报文日志，保留 7 天冷存储，Kafka header 透传。
6. **白标化**：前端不写死品牌，进入时按 AppId/域名请求 `/api/tenant/config` 下发主题色、Logo、支付商户参数；加本地缓存 + 默认值降级。

## 部署形态
- **形态一**：≤1000 把枪，2 台 8核16G，docker-compose.yml + .env + install.sh，15 分钟拉起。
- **形态二**：数万把枪，K8s + Helm；SLB 四层 TCP（Proxy Protocol 透传桩IP）+ 七层 HTTPS/WSS；IoT 网关 HPA + 优雅下线（建议改为"下发重连指令+限期断开"，避免等长订单卡死发版）；数据层主从 + Redis Cluster/Sentinel + TDengine 3 节点。

## 待用户确认的决策点
- Q1：金额 Go 语境选 int64 定点 scale=4（建议）还是 shopspring/decimal？
- Q2：IoT 优雅下线用"下发重连指令"替代"等订单自然结束"？建议是。
- Q3：core-service 模块化单体 vs 独立微服务？建议先单体。
- Q4：前端是否纳入本仓库？建议单开仓库关联。
- Q5：后端 Go 已提出待确认；构建改 go.work + go.mod。
- 用户回复 "go" 后才执行创建/推送。

## 环境
- GitHub CLI 已登录 SionFu，token 有 repo 权限；git identity = Joe <fu_sion@live.com>。
- 本地已有 fork `SionFu/HUIZHI-ChargeOS-cloud`（同类项目，可参考不复制）。
