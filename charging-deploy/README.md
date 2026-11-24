# charging-deploy

私有化交付包，提供两种标准形态。

## 形态一：敏捷私有化（≤1000 把枪）

```bash
cp .env.example .env
# 编辑 .env：域名、SSL 证书路径、微信商户号
./install.sh
```

15 分钟内拉起全套环境。

## 形态二：高可用集群（数万把枪）

```bash
helm install opencharge ./helm -f values.yaml
```

- SLB 四层 TCP 转发桩流量（Proxy Protocol 透传真实 IP）
- 七层 HTTPS/WSS 转发小程序 API 与 OCPP
- IoT 网关 HPA 基于连接数与 CPU 扩缩容
- 滚动更新：停接新连接 → 下发重连 → 限期断开
- 数据层：数据库主从 + Redis Cluster + TDengine 3 节点
