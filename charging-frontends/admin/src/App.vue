<script setup>
import { ref, onMounted } from 'vue'
import { loadTenantConfig } from '@opencharge/shared'
import Orders from './views/Orders.vue'
import Exceptions from './views/Exceptions.vue'
import Audits from './views/Audits.vue'

const config = ref(null)
const tab = ref('orders')
const apiBase = import.meta.env.VITE_API_BASE || ''

onMounted(async () => {
  config.value = await loadTenantConfig({ domain: window.location.hostname, apiBase })
})
</script>

<template>
  <div class="app">
    <header>
      <h1>{{ config?.name || 'OpenCharge' }} 运营后台</h1>
      <span class="tag">金额精度 int64 · 交叉校验</span>
    </header>
    <nav>
      <button :class="{active: tab === 'orders'}" @click="tab = 'orders'">订单</button>
      <button :class="{active: tab === 'exceptions'}" @click="tab = 'exceptions'">异常单</button>
      <button :class="{active: tab === 'audits'}" @click="tab = 'audits'">容差审计</button>
    </nav>
    <main>
      <Orders v-if="tab === 'orders'" :api-base="apiBase" />
      <Exceptions v-else-if="tab === 'exceptions'" :api-base="apiBase" />
      <Audits v-else :api-base="apiBase" />
    </main>
  </div>
</template>

<style>
* { box-sizing: border-box; }
body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #f5f6f8; color: #1f2329; }
.app header { display: flex; align-items: center; justify-content: space-between; background: #fff; padding: 16px 24px; border-bottom: 1px solid #e5e6eb; }
.app header h1 { font-size: 18px; margin: 0; }
.tag { font-size: 12px; color: #86909c; background: #f2f3f5; padding: 4px 10px; border-radius: 12px; }
nav { display: flex; gap: 8px; padding: 12px 24px; background: #fff; }
nav button { border: 1px solid #e5e6eb; background: #fff; padding: 8px 18px; border-radius: 6px; cursor: pointer; font-size: 14px; }
nav button.active { background: #1677ff; color: #fff; border-color: #1677ff; }
main { padding: 20px 24px; }
table { width: 100%; border-collapse: collapse; background: #fff; border-radius: 8px; overflow: hidden; }
th, td { text-align: left; padding: 10px 14px; border-bottom: 1px solid #f0f1f2; font-size: 13px; }
th { background: #fafafa; color: #4e5969; font-weight: 500; }
.state-completed { color: #00a870; }
.state-exception { color: #f53f3f; }
.amount { font-variant-numeric: tabular-nums; font-family: ui-monospace, monospace; }
.error { color: #f53f3f; padding: 12px; }
</style>
