# 最小测试闭合（MVP Closed Loop）开发计划

> 目标：用**一条 `go test`**跑通「扫码 → 启动 → 遥测 → 停机 → 计价 → 扣款 → 完成」全链路，全程内存假件，不依赖 Redis / Kafka / MySQL / 真实硬件。这条测试通过，即证明架构的六个硬约束（金额精度、枪头锁、快慢通道、状态机、电价快照、TraceId）在真实代码路径上成立，而不只是各模块单测成立。

---

## 一、当前为什么跑不通

核对现有代码，闭环缺的是**四个端口（interface）和一个编排层**：

| 缺口 | 现状 | 后果 |
|---|---|---|
| `lock.ConnectorLock` | 直接绑定 `*redis.Client` 的具体类 | 测试必须起 Redis，否则无法验证枪头锁 |
| 事件总线 | 只有 Topic 字符串常量，没有 Publish/Subscribe 接口 | 网关解码后无处投递，业务侧收不到事件 |
| 订单存储 | `Order` 是结构体，无 Store 接口 | 状态机无法持久化、无法断言最终状态 |
| 钱包 / 电价存储 | `Wallet`、`TariffVersion` 无 Store 接口 | 扣款与取价无法在测试中替换 |
| 编排层 | 适配器只做 `Decode`，没有「解码→分流→消费→结算」的串联 | 各模块各自为战，没有端到端路径 |

**结论**：先补端口（Ports & Adapters），再写编排，最后写闭合测试。这是唯一能保证「测试的是真实代码路径，而不是测试专用替身」的做法。

---

## 二、目标闭环场景（Done 的定义）

### 正向场景：一次完整充电

| 步骤 | 输入事件 | 期望行为 | 断言点 |
|---|---|---|---|
| 1 | `BootNotification(DEV001)` | 网关注册设备路由 | TraceId 生成 |
| 2 | 用户扫码 `DEV001:1` | 加枪头锁 → 建预占订单 `created` → 下发启动指令 | 锁 key = `lock:connector:DEV001:1` |
| 3 | `StatusNotification=Charging` | 状态机 `starting → charging` | 合法流转 |
| 4 | `MeterValues` ×2 | 走**快通道** Topic，累计电量 | 未进入高优 Topic |
| 5 | `StopTransaction(Offline=false, tariffVersionId=V1)` | 走**高优** Topic，触发计费+扣款 | 见下方精确金额 |
| 6 | 结算完成 | 订单 `completed`，释放枪头锁，钱包已扣款 | 锁已释放 |

### 精确金额断言（核心中的核心）

电价快照 `V1`：谷 `0.3000` 元/kWh、峰 `0.6000` 元/kWh。

| 分段 | 电量 | 单价 | 小计 |
|---|---|---|---|
| 谷 | 1500 Wh | 0.3000 | 0.4500 元 |
| 峰 | 500 Wh | 0.6000 | 0.3000 元 |
| **合计** | 2000 Wh | — | **0.7500 元 → 结算 0.75 元** |

钱包初始本金 `10.0000` 元 → 扣款后 `9.2500` 元。全程 `int64` 定点，零浮点。

### 反向场景（缺一不可）

| 场景 | 输入 | 期望 |
|---|---|---|
| 重复扫码 | 同一把枪并发扫码两次 | 第二次加锁失败，**不下发第二条启动指令** |
| 乱序状态 | `created` 直接收到 `Charging` | 状态机拒绝，订单不变 |
| 离线补单 | `StopTransaction(Offline=true, tariffVersionId=V1)`，且当前价已改为 V2 | 按 **V1 历史价**计费，不按 V2 |

---

## 三、分阶段任务清单

### 阶段 1：补端口（Ports）—— 让一切可替换

| # | 任务 | 文件 | 验收 |
|---|---|---|---|
| 1.1 | 抽出 `Locker` 接口，Redis 实现改名为 `RedisLocker` | `charging-common/lock/locker.go`、`connector.go` | `go build` 通过 |
| 1.2 | 新增内存锁 `MemoryLocker`（map + mutex，支持并发竞争与 TTL 语义） | `charging-common/lock/memory.go` | 单测：第二次 Acquire 失败 |
| 1.3 | 新增事件总线端口 `Bus` + `MemoryBus`（按 topic 分发，记录已投递事件供断言） | `charging-common/bus/bus.go`、`memory.go` | 单测：MeterValues 只进快通道 |
| 1.4 | 新增 `OrderStore` 接口 + 内存实现 | `charging-core-service/module-order-billing/store.go` | 编译通过 |
| 1.5 | 新增 `TariffStore`、`WalletStore` 接口 + 内存实现 | `module-order-billing/tariff_store.go`、`module-payment-clearing/wallet_store.go` | 编译通过 |

### 阶段 2：编排层 —— 把模块串成路径

| # | 任务 | 文件 | 验收 |
|---|---|---|---|
| 2.1 | 网关流水线：`frame → adapter.Decode → TraceId → Bus.Publish`，按 `channels.IsHighPriority` 分流 | `charging-iot-gateway/internal/pipeline/pipeline.go` | 单测：解码后事件进入正确 Topic |
| 2.2 | 订单服务：`Start()` 持枪头锁建单；`OnStatus()` 推进状态机；`OnStop()` 触发结算 | `module-order-billing/service.go` | 单测：状态机合法/非法流转 |
| 2.3 | 结算服务：取电价快照 → `CalculateFee` → `Wallet.Debit` → 订单完成 → 释放锁 | `module-order-billing/settlement.go` | 单测：金额 0.75、余额 9.25 |
| 2.4 | 钱包扣款方法（本金优先、余额不足拒绝） | `module-payment-clearing/wallet.go` 补 `Debit` | 单测通过 |

### 阶段 3：闭合测试 —— 一条命令跑通

| # | 任务 | 文件 | 验收 |
|---|---|---|---|
| 3.1 | 新增独立测试模块，依赖全部四个模块 | `charging-integration/go.mod`，加入 `go.work` | `go work sync` 通过 |
| 3.2 | 正向端到端测试 `TestFullChargeLoop` | `charging-integration/loop_test.go` | 六步全断言通过，金额精确 |
| 3.3 | 反向测试：重复扫码 / 乱序状态 / 离线补单 | `charging-integration/negative_test.go` | 三个场景全通过 |
| 3.4 | 测试辅助：构造云快充帧、电价快照、钱包 | `charging-integration/fixture.go` | — |

### 阶段 4：工程化收尾

| # | 任务 | 验收 |
|---|---|---|
| 4.1 | `Makefile` 增加 `make test` 一键跑全部模块 + 集成测试 | 一条命令全绿 |
| 4.2 | 补 `docs/mvp-plan.md` 到 README 链接 | 文档可发现 |
| 4.3 | （可选）GitHub Actions：push/PR 触发 `go test` | CI 绿灯 |

---

## 四、为什么这样设计

- **端口先行**：如果直接写测试、在测试里 new 一个假 Redis，测的是替身不是真实代码。抽出接口后，生产走 Redis/Kafka，测试走内存，跑的是**同一条编排路径**。
- **独立集成模块**：`core-service` 不该反向依赖 `iot-gateway`（方向错误）。单独建 `charging-integration` 测试模块依赖四方，既保持依赖方向干净，又符合 Monorepo 习惯。
- **金额硬断言**：闭合测试里写出 `0.7500 → 0.75` 的精确期望值，任何一次浮点回归、舍入错误、电价版本错用都会立刻红灯。

---

## 五、需要你拍板的决策点

1. **集成测试模块**：新增第 5 个模块 `charging-integration`（建议）？还是把闭合测试塞进 `charging-core-service`（省一个模块，但 core 要依赖 gateway 的适配器，依赖方向略脏）？
2. **测试范围**：本轮只做「云快充 + 在线正向 + 三个反向」最小集（建议），还是同时把 OCPP 也纳入闭合？
3. **是否直接实现**：确认后我就按阶段 1→4 逐个落地，每阶段跑一遍测试验证再进下一步。
