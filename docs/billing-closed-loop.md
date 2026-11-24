# 计费闭环实现说明（阶段 1 + 2）

> 2026-11-25 落地。数据落库 + 交叉校验计费 + 后台查看的最小闭环。

## 端到端链路

```
桩端 0x3B 账单 → YKC TCP 网关 → SettlementBridge 转换 Bill
    → SettlementService：幂等 → 硬排除 → 平台重算 → 容差判定 → 钱包扣款 → 订单 completed
    → 内存/PostgreSQL 存储 → admin REST API → Vue3 后台
```

网关是 MVP 的 composition root：TCP 服务、结算引擎、admin API 在同一进程共享存储，因此模拟器跑完一笔账单后立刻能在后台看到订单。

## 计费决策（已实现，全部整数运算）

设 `P` = 平台按电价快照重算金额、`D` = 桩端消费金额（scale=4，1 单位 = 0.0001 元）。

**硬排除（直接挂异常单，绝不扣款）**：

- 桩端四段金额之和 ≠ 消费金额
- 电量为负或分段电量与总电量偏差 >1 单位
- 金额超过自动结算上限（默认 500 元）
- 找不到对应电价版本快照 / 快照租户场站不匹配 / 分段时段在快照中无价格

**软容差**（`money.DefaultTolerance`）：`|P−D| ≤ min(0.01 元, 0.1% × 订单金额)` 时自动结算，记审计；否则挂异常单。容差内按 `min(P, D)` 扣款，保证用户不会比桩端屏显多付。

**电价精度**：协议电价 5 位小数，新增 `money.Price`（scale=5）；金额仍用 `money.Money`（scale=4）。`Price.MulEnergy(kWh scale=4)` 返回 `Money`，整数 half-up。

**幂等**：`charging_order.transaction_id` 唯一；钱包扣款用 `applied_debit` 表按 transactionId 去重。0x3B 重传不会重复扣款。

## 存储

- 端口定义在 `charging-core-service/storage`：`OrderStore` / `TariffStore` / `WalletStore` / `AuditStore`
- 两套实现：内存（测试与本地演示）、GORM + PostgreSQL（`-dsn` 启用）
- 新增表：`order_segments`、`billing_audit`、`exception_order`、`applied_debit`，见 `charging-deploy/sql/init.sql`

## 运行

```bash
cd charging-iot-gateway
CGO_ENABLED=0 go build -o gateway ./cmd/gateway
CGO_ENABLED=0 go build -o sim ./device-simulator/ykc

./gateway -addr :8768 -admin :8081      # 内存演示模式，内置电价 V1 与钱包 U1
./sim -addr 127.0.0.1:8768              # 另开终端，跑一笔 0.5kWh 账单

curl http://localhost:8081/api/admin/orders
curl http://localhost:8081/api/admin/audits
curl http://localhost:8081/api/admin/exceptions
```

前端管理页在 `charging-frontends/admin`，`npm run dev`，通过 `VITE_API_BASE` 指向 admin API。

## 尚未实现（后续阶段）

- Redis 实时枪态与分布式锁持久化
- TDengine 遥测曲线落库
- Kafka 快慢通道解耦（当前进程内分流）
- 电价版本管理后台页（当前通过存储接口写入）

---

## 阶段 3+4：中间件可切换实现（2026-11-25 追加）

三种中间件都采用**端口 + 双实现**，默认内存路径保持已验证的闭环不变：

| 能力   | 端口                   | 默认实现          | 生产实现                                    | 启用参数                         |
| ---- | -------------------- | ------------- | --------------------------------------- | ---------------------------- |
| 实时枪态 | `common/state.Store` | `MemoryStore` | `RedisStore`（TTL 5 分钟自动过期）              | `-redis host:port`           |
| 遥测落库 | `telemetry.Store`    | `MemoryStore` | `TDengineStore`（REST `/api/v2/sql`，批量写） | `-tdengine http://host:6041` |
| 事件分流 | `bus.Bus`            | `MemoryBus`   | `KafkaBus`（原生 Produce v3 协议，零第三方依赖）     | `-kafka host:port`           |

**关键设计**：

- Kafka 通道按 `channels.TopicTelemetry` / `TopicState` 物理隔离，状态主题用 `{deviceId}:{gunNo}` 作分区键保证同枪有序；原生协议实现带 CRC32C 与 trace 头。
- 遥测经 `Buffer` 按数量（50 条）或时间（3 秒）批量刷写；Push 非阻塞，不拖慢桩端读取与结算。
- 枪态更新与结算通过 `CompositeHandler` 串联，状态先更新、结算后执行；停机时保留最后一次电量/金额供后台查看。
- admin 新增 `/api/admin/connectors` 查询实时枪态。

**验证情况**：

- 默认路径：四模块 build/test/vet 全绿；真实 TCP 模拟器 → 结算 settled → admin 订单与枪态可见（回归通过）。
- Redis：用 Docker 容器实测，枪态写入并可通过 admin API 查询。
- TDengine：容器在本沙箱因内存限制无法启动，改用 HTTP mock 验证建库/建表/批量 INSERT 的 SQL 与数值正确。
- Kafka：容器同样受内存限制，改用协议级单测验证 Produce 请求结构、分区键、trace 头、变长编码与 CRC32C 已知向量（发现并修复了大端序与 CRC 偏移两个真实 bug）。

---

## 电价版本管理（2026-11-25 追加）

**不可变快照原则**：电价版本一经发布不可修改，变更即新建版本；历史订单的 `tariffVersionId` 始终指向当时的价格快照，重算/回溯不会受后续改价影响。

**API**（`module-admin/tariff_handler.go`）：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/admin/tariffs?tenantId=&stationId=` | 版本列表，按生效时间倒序 |
| GET | `/api/admin/tariffs/{id}` | 版本详情 |
| POST | `/api/admin/tariffs` | 新建版本；ID 由 `tenant-station-生效unix秒` 生成，同输入幂等 |

**校验**（`storage.ValidateTariff`）：必须恰好 48 个时段、slot 在 0..47、无重复、价格非负。价格用字符串传输（如 `"0.62345"`），避免 JSON 浮点丢精度。

**前端**（`charging-frontends/admin/src/views/Tariffs.vue`）：48 时段网格编辑器、尖峰平谷批量填充、5 位小数格式校验、历史版本一键载入再发布。

**验证**：API 单测覆盖创建/列表/详情/47 时段拒绝/重复 slot 拒绝/非法价格拒绝；前端 `vite build` 通过；实测通过 API 发布 0.90000 版本后，同一笔账单平台重算为 0.4500、与桩端 0.3000 差额超容差，正确挂起为异常单——证明新版本确实驱动了结算。

---

## 异常单人工处置 + 订单分时明细（2026-11-25 追加）

**此前的缺口**：`POST /api/admin/exceptions/resolve` 只把 `resolved` 置为 true，**钱没有动**，异常单永远无法变成已结算收入——闭环在异常分支上是断的。

**现在的处置流程**（`SettlementService.ResolveException`）：

| 决策 | 扣款金额 | 说明 |
|---|---|---|
| `platform` | 平台重算金额 | 平台权威 |
| `pile` | 桩端上报金额 | 采信设备 |
| `custom` | 人工输入 | 协商折扣，必须传 `amount` |

一次处置会：幂等扣款 → 订单 `exception → completed` → 写审计（`rule=manual_<decision>`）→ 标记异常单已处理。重复处置**不会二次扣款**，并返回真实钱包余额。

**为支持处置补的字段**：`Order.UserID`——此前订单没记钱包归属，异常单根本找不到该扣谁的款。

**订单分时明细**：`GET /api/admin/orders/{transactionId}` 返回订单 + `segments`（尖/峰/平/谷的单价、电量、金额）。异常单也会保存分段，便于运营核对；此前异常单只有总额没有明细。

**前端**：订单行可展开查看分时明细；异常单页改为真实处置（下拉选择平台/桩端/自定义金额），处置后提示扣款金额与余额。

**实测**：模拟器以 0.9 元计价产生异常单（桩端 0.4500 / 平台 0.3000）→ 自定义 0.35 处置 → 钱包 10.00 → 9.35，订单转 completed，异常单标记完成，重复处置余额仍为 9.35（未二次扣款）。
