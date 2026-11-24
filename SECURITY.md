# 安全策略

## 支持的版本

当前 main 分支。

## 报告漏洞

请勿公开提交 Issue。发送邮件至 security@rarhetec.example，标题格式 `[OpenCharge] 漏洞简述`。我们会在 48 小时内响应。

## 安全设计要点

- 设备一机一密（DeviceSecret / X.509 双向 TLS）
- 指令防重放：CommandSeq 递增 + 时间戳窗口（<30s）
- 敏感字段落库 AES-256 加密，展示层脱敏
- 数据导出二次验证 + 水印审计
- 资金不沉淀：走服务商分账或运营商直连
