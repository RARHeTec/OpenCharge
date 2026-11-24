# charging-core-service

核心业务域，当前为**模块化单体**（module 之间只通过接口与统一事件通信，不互相 import 内部包），可在规模上来后按 module 边界拆成独立服务。

## 模块

| 模块 | 职责 |
|---|---|
| `module-iam` | 自研 OAuth2.1、多租户组织树、B2B2C 客户授权凭证；预留外部 IdP 扩展口 |
| `module-device` | 场站 / 电桩 / 枪头台账、远程运维与诊断 |
| `module-order-billing` | 充电生命周期状态机、尖峰平谷 48 时段计费、电价版本快照 |
| `module-payment-clearing` | 钱包账户、服务商分账路由、T+1 三方对账 |
| `module-openapi` | 第三方运营商 / 车队 ERP 对接开放平台 |

## 铁律

- 所有金额使用 `charging-common/money.Money`，代码里出现 float64 金额即视为 bug
- 所有查询带 `tenant.Scope(ctx)`，云快充启动前必须持枪头锁
- 离线补单按 `tariffVersionId` 取历史电价，禁止用当前价回溯
