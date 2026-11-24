# ADR-0005: 后端采用 Go，前端 Vue 3 + uni-app

- 状态：Accepted
- 日期：2026-11-24

## 背景

原规划文档推荐 Java 17 + Spring Cloud。用户提出 Go。

## 决策

后端 Go 1.22：IoT 网关用 goroutine 处理长连接，协议 SPI 用 interface + registry，构建用 go.work。前端 Vue 3（管理后台）+ uni-app（小程序），同仓分目录。IAM 先 Go 自研，预留 ExternalIdP 扩展口。

## 后果

- 优点：网关高并发更省资源、前后端同仓协作方便
- 代价：无 Spring 生态，租户隔离与 OAuth2.1 需自研；前端拆包节奏分化后再拆仓
