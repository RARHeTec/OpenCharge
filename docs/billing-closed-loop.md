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
