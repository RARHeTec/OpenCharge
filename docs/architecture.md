# OpenCharge 系统架构

> 源自《私有充电平台（SaaS/PaaS）系统架构与从零开发规划文档》，按 Go 技术栈落地。

## 1. 项目定位

支持"平台自营 + 运营商多租户加盟 + 第三方互联互通"的私有化充电运营平台，兼顾 PaaS 底座通用扩展能力。

五层架构：

| 层级 | 组件 |
|---|---|
| 多端应用交互层 | uni-app 小程序 / Vue 3 管理后台，C 端支持品牌白标 |
| 统一认证与网关层 | OAuth2.1、RBAC+ABAC、API 网关 |
| 平台核心业务层 | 设备中心、订单计费、支付清分、开放平台 |
| 数据与存储层 | PostgreSQL、TDengine、Redis、Kafka |
| 终端与边缘接入层 | Go net / EMQX，长连接高并发网关 |

## 2. 多协议兼容

**统一物模型**是网关与业务之间唯一语言：BootNotification、Heartbeat、StatusNotification、MeterValues、RemoteStart/StopTransaction、SetChargingProfile、ReserveNow、TriggerMessage、Diagnostics。

**插件化协议适配**（SPI）：云快充 1.5/1.6、OCPP 1.6J、国网 104、私有 TCP/MQTT。适配层把报文翻译成统一事件投递到 Kafka。

连接路由：网关无状态，Redis 维护 `DeviceId -> {NodeIP, SessionId}` 映射。指令带全局唯一 TransactionId 与递增 CommandSeq，Redis 分布式锁防重复。

## 3. 充电生命周期与计费

扫码 → 鉴权（余额/黑名单/运营商授权/预约检查）→ 创建预占订单 → 下发 RemoteStart → Ack 后转 Charging → 15~30s MeterValues 遥测 → 风控（预付费阈值 / BMS 告警）→ StopTransaction → 终结算 → 释放枪锁。

计费支持站级 > 桩级 > 运营商级 > 客户集团级覆盖，每天最多 48 个半小时时段（尖/峰/平/谷）。费用由电费 + 服务费 + 停车费累加。老旧桩走时间片积分估算。

**弱网补偿**：桩端离线凭白名单充电，恢复后补传 Offline Transaction；订单冗余 `tariff_version_id`，按历史电价快照回溯计费。

## 4. 支付与清分

模式 A：平台大商户 + 服务商分账（92% 运营商 / 5% 平台 / 3% 场地），适合中小运营商。
模式 B：运营商直连商户号，平台月度出 SaaS 服务费账单，适合大型连锁。

支付方式：预充值钱包（本金/赠送金隔离）、先充后付免密、B 端车队授信、即插即充/IC 卡。

T+1 三方对账：渠道流水 / 平台订单 / 桩端账单勾稽，异常分为长款、短款、金额不符，自动生成工单。

## 5. IAM 与客户授权

组织树：Platform Root → Partner → Operator(Tenant) → Station Admin → Role。数据隔离在 SQL 层注入 `tenant_id` 与 `station_id`。

B2B2C 双层账号：Global User Profile（手机号、实名、VIN、通用钱包）+ Tenant User Profile（会员等级、专有券、隐私授权）。首次扫码生成 User-Tenant-Grant，仅授权客户可见。平台通用资产可漫游，运营商私域资产严格隔离。敏感字段脱敏展示，导出需 OTP 二次验证 + 水印。

## 6. 技术选型

Go 1.22+ / Vue 3 + uni-app / Go net + EMQX / PostgreSQL / TDengine / Redis / Kafka / Docker + K8s。

## 7. 落地路线

- 阶段一（1-6 周）：IAM 底座、统一物模型、单协议闭环、基础计费、扫码支付
- 阶段二（7-12 周）：多协议扩展、自动分账、免密支付、独立后台、T+1 对账
- 阶段三（13-18 周）：跨运营商漫游、车队授信、有序充电、智能运维

## 8. 风控安全

- 资金二清合规：不设资金池，走分账或直连
- BMS 故障秒级断电，网关绕过业务层直接下发停机
- 指令防重放：CommandSeq + 时间戳窗口 <30s
- AES-256 列级加密，PIPL/GDPR 合规
- 设备一机一密（DeviceSecret / mTLS X.509）
