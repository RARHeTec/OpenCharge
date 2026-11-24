# 贡献指南

## 开发环境

- Go 1.22+
- Node.js 20+（仅前端）
- Docker / Docker Compose（本地联调）

## 提交规范

```
feat: 新功能
fix: 修复缺陷
docs: 文档
refactor: 重构
test: 测试
chore: 工程化
```

## 硬性要求

1. 禁止提交浮点金额运算，所有账务走 `charging-common/money`
2. 新增查询必须支持多租户隔离，使用 `tenant.Scope(ctx)`
3. 启动/停机指令必须持枪头锁
4. 变更协议或事件结构时同步更新 `charging-thing-model` 与 ADR
5. 提交前执行 `go build ./...` 与 `go test ./...`

## Pull Request

Fork → 分支 → 提交 → PR。请确保 CI 通过并说明变更影响面。
