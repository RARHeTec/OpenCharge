<script setup>
import { ref, onMounted } from 'vue'

const props = defineProps({ apiBase: String })
const rows = ref([])
const error = ref('')
const open = ref({})     // txn -> bool
const details = ref({})  // txn -> segments

async function load() {
  try {
    const res = await fetch(`${props.apiBase}/api/admin/orders`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const json = await res.json()
    rows.value = json.data || []
    error.value = ''
  } catch (e) {
    error.value = `加载失败：${e.message}`
  }
}

async function toggle(o) {
  const txn = o.transactionId
  if (open.value[txn]) {
    open.value[txn] = false
    return
  }
  try {
    const res = await fetch(`${props.apiBase}/api/admin/orders/${encodeURIComponent(txn)}`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const json = await res.json()
    details.value[txn] = json.segments || []
    open.value[txn] = true
  } catch (e) {
    error.value = `加载明细失败：${e.message}`
  }
}

const periodName = { sharp: '尖', peak: '峰', flat: '平', valley: '谷' }

onMounted(load)
</script>

<template>
  <section>
    <h2>充电订单</h2>
    <p v-if="error" class="error">{{ error }}</p>
    <table>
      <thead>
        <tr>
          <th></th><th>交易流水号</th><th>设备/枪</th><th>状态</th>
          <th>桩端金额</th><th>平台金额</th><th>实收</th><th>电价版本</th>
        </tr>
      </thead>
      <tbody>
        <template v-for="o in rows" :key="o.transactionId">
          <tr>
            <td><button class="expand" @click="toggle(o)">{{ open[o.transactionId] ? '−' : '+' }}</button></td>
            <td class="mono">{{ o.transactionId.slice(0, 12) }}…</td>
            <td>{{ o.deviceId }} / {{ o.gunNo }}</td>
            <td :class="`state-${o.state}`">{{ o.state }}</td>
            <td class="amount">{{ o.pileAmount }}</td>
            <td class="amount">{{ o.platformAmount }}</td>
            <td class="amount">{{ o.chargeAmount }}</td>
            <td class="mono small">{{ o.tariffVersionId }}</td>
          </tr>
          <tr v-if="open[o.transactionId]" class="detail-row">
            <td colspan="8">
              <div class="detail">
                <div class="meta">
                  电量 {{ (o.energyUnits / 10000).toFixed(4) }} kWh ·
                  用户 {{ o.userId || '—' }} ·
                  {{ o.offline ? '离线补单' : '在线' }}
                  <span v-if="o.exceptionReason"> · 备注 {{ o.exceptionReason }}</span>
                </div>
                <table class="inner">
                  <thead><tr><th>时段</th><th>单价(元/kWh)</th><th>电量(kWh)</th><th>金额</th></tr></thead>
                  <tbody>
                    <tr v-for="(s, i) in (details[o.transactionId] || [])" :key="i">
                      <td>{{ periodName[s.period] || s.period }}</td>
                      <td class="amount">{{ s.price }}</td>
                      <td class="amount">{{ (s.energyUnits / 10000).toFixed(4) }}</td>
                      <td class="amount">{{ s.amount }}</td>
                    </tr>
                    <tr v-if="!(details[o.transactionId] || []).length"><td colspan="4">无分时明细</td></tr>
                  </tbody>
                </table>
              </div>
            </td>
          </tr>
        </template>
        <tr v-if="!rows.length"><td colspan="8">暂无订单</td></tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.expand { width: 24px; height: 24px; border: 1px solid #e5e6eb; background: #fff; border-radius: 4px; cursor: pointer; line-height: 1; }
.detail-row td { background: #fafbfc; }
.detail { padding: 8px 4px; }
.meta { font-size: 12px; color: #4e5969; margin-bottom: 8px; }
.inner { width: auto; min-width: 420px; background: #fff; border: 1px solid #f0f1f2; }
.inner th { font-size: 12px; }
.mono { font-family: ui-monospace, monospace; font-size: 12px; }
.small { font-size: 11px; }
</style>
