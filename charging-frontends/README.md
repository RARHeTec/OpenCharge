# charging-frontends

前端同仓分目录，三个包共享类型与工具，待研发和发版节奏分化后再拆仓发共享包。

```
charging-frontends/
├── admin/      # Vue 3 + Vite 管理后台
├── uni-app/    # uni-app C 端小程序（微信/支付宝）
└── shared/     # 共享类型、API client、主题配置降级逻辑
```

## 白标化

`shared` 中的 `loadTenantConfig()` 按 AppId 或域名请求 `/api/tenant/config`，带本地缓存 + 默认值降级。任何端都不得写死品牌 Logo / 名称 / 客服电话。
