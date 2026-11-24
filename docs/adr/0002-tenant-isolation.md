# ADR-0002: 多租户隔离采用 GORM Scope + fail-closed

- 状态：Accepted
- 日期：2026-11-24

## 背景

Go 没有 MyBatis-Plus 的 SQL 拦截器。文档要求 SQL 层自动注入 `tenant_id`，运营商内部还要按 `station_id` 收窄。

## 决策

`charging-common/tenant` 提供 `Scope(ctx)`、`StationScope(ctx)`、`Scoped(ctx)`。上下文缺失或租户 ID 为空时注入 `WHERE 1 = 0`（fail-closed），绝不当成"全租户"。

## 后果

- 优点：集中管控、漏写即拒绝而非泄漏
- 代价：所有查询必须显式挂 Scope，CR 需检查
