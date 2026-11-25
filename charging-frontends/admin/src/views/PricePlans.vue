<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import PricePlanDialog from './PricePlanDialog.vue'

const props = defineProps({ apiBase: String })
const tenantId = ref('T1')
const stationId = ref('S1')
const view = ref('current') // current | history
const deviceTypes = ref([])
const selectedType = ref('')
const current = ref([])
const history = ref([])
const error = ref('')
const notice = ref('')
const showDialog = ref(false)

function req(path, opts) {
  return fetch(`${props.apiBase}${path}`, opts).then(async (res) => {
    const json = await res.json().catch(() => ({}))
    if (!res.ok) throw new Error(json.error || `HTTP ${res.status}`)
    return json
  })
}

async function load() {
  try {
    const q = `tenantId=${encodeURIComponent(tenantId.value)}&stationId=${encodeURIComponent(stationId.value)}`
    const [t, c, h] = await Promise.all([
      req(`/api/admin/device-types?tenantId=${encodeURIComponent(tenantId.value)}`),
      req(`/api/admin/price-plans?${q}&view=current`),
      req(`/api/admin/price-plans?${q}&view=history`),
    ])
    deviceTypes.value = t.data || []
    current.value = c.data || []
    history.value = h.data || []
    if (!selectedType.value && deviceTypes.value.length) {
      selectedType.value = deviceTypes.value[0].name
    }
    error.value = ''
  } catch (e) {
    error.value = `加载失败：${e.message}`
  }
}

const activePlans = computed(() => {
  if (!selectedType.value) return current.value
  return current.value.filter((p) => p.deviceType === selectedType.value)
})

const delivered = computed(() => activePlans.value.reduce((n, p) => n + (p.deliveredCount || 0), 0))
const succeeded = computed(() => activePlans.value.reduce((n, p) => n + (p.successCount || 0), 0))
const activePlan = computed(() => activePlans.value[0] || null)

const fmtPrice = (v) => {
  const s = String(v || '0')
  const [i, f = ''] = s.split('.')
  return `${i}.${f.padEnd(4, '0').slice(0, 4)}`
}
const totalOf = (seg) => {
  const sum = (Number(seg.elecPrice) || 0) + (Number(seg.servicePrice) || 0)
  return sum.toFixed(4)
}
const periodLabel = (p) => ({ sharp: '尖', peak: '峰', flat: '平', valley: '谷' }[p] || p)
const periodClass = (p) => ({ sharp: 'sharp', peak: 'peak', flat: 'flat', valley: 'valley' }[p] || 'flat')
const timeOf = (plan) => plan?.createdAt ? new Date(plan.createdAt).toLocaleString('zh-CN', { hour12: false }) : '—'

onMounted(load)
watch([tenantId, stationId], load)
</script>

<template>
  <section class="page">
    <div class="head">
      <h2>设置定价</h2>
      <div class="segmented">
        <button :class="{ on: view === 'current' }" @click="view = 'current'">当前价格</button>
        <button :class="{ on: view === 'history' }" @click="view = 'history'">历史定价</button>
      </div>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="notice" class="notice">{{ notice }}</p>

    <div class="toolbar">
      <label>租户 <input v-model="tenantId" @change="load" /></label>
      <label>场站 <input v-model="stationId" @change="load" /></label>
      <button class="primary" @click="showDialog = true">调整定价</button>
      <button class="plain">柜下发记录</button>
    </div>

    <template v-if="view === 'current'">
      <h3 class="sub">当前价格</h3>
      <div class="cards">
        <button v-for="t in deviceTypes" :key="t.name" class="card"
                :class="{ active: selectedType === t.name }" @click="selectedType = t.name">
          <span class="card-name">{{ t.name }}</span>
          <span class="card-count">({{ t.count }})</span>
        </button>
      </div>
      <p class="delivered">共下发 {{ delivered }} 个桩，下发成功 {{ succeeded }}；</p>

      <table>
        <thead>
          <tr><th>序号</th><th>时间段</th><th>时段</th><th>电费单价 (元/kWh)</th><th>服务费单价 (元/kWh)</th><th>总单价 (元/kWh)</th></tr>
        </thead>
        <tbody>
          <tr v-for="seg in activePlan?.segments || []" :key="seg.seq">
            <td>{{ seg.seq }}</td>
            <td class="mono">{{ String(Math.floor(seg.startMin/60)).padStart(2,'0') }}:{{ String(seg.startMin%60).padStart(2,'0') }}~{{ String(Math.floor(seg.endMin/60)).padStart(2,'0') }}:{{ String(seg.endMin%60).padStart(2,'0') }}</td>
            <td><span class="period" :class="periodClass(seg.period)">{{ periodLabel(seg.period) }}</span></td>
            <td class="num">{{ fmtPrice(seg.elecPrice) }}</td>
            <td class="num">{{ fmtPrice(seg.servicePrice) }}</td>
            <td class="num total">{{ totalOf(seg) }}</td>
          </tr>
          <tr v-if="!activePlan"><td colspan="6" class="empty">暂无当前价格，点「调整定价」创建</td></tr>
        </tbody>
      </table>
    </template>

    <template v-else>
      <h3 class="sub">历史定价</h3>
      <table>
        <thead>
          <tr><th>生效时间</th><th>场站计费</th><th>设备类型</th><th>价格状态</th><th>电价来源</th><th>电价说明</th><th>操作人</th><th>操作</th></tr>
        </thead>
        <tbody>
          <tr v-for="p in history" :key="p.id">
            <td class="mono small">{{ timeOf(p) }}</td>
            <td class="mono">{{ p.stationId }}</td>
            <td>{{ p.deviceType }}</td>
            <td><span class="status" :class="p.status">{{ p.status === 'valid' ? '有效' : '无效' }}</span></td>
            <td>{{ p.source === 'manual' ? '人工定价' : p.source }}</td>
            <td>{{ p.note || p.source === 'manual' ? (p.note || '人工定价') : '—' }}</td>
            <td>{{ p.operator }}</td>
            <td class="ops"><button class="link">详情</button><button class="link">柜下发记录</button></td>
          </tr>
          <tr v-if="!history.length"><td colspan="8" class="empty">暂无历史定价</td></tr>
        </tbody>
      </table>
      <p class="pager">共 {{ history.length }} 条记录</p>
    </template>

    <PricePlanDialog v-if="showDialog" :api-base="apiBase" :tenant-id="tenantId"
                     :station-id="stationId" :device-types="deviceTypes"
                     :initial-plan="activePlan" @close="showDialog = false"
                     @saved="(msg) => { notice = msg; showDialog = false; load() }" />
  </section>
</template>


<style scoped>
.page { padding: 4px 0; }
.head { display: flex; justify-content: space-between; align-items: center; }
h2 { font-size: 16px; margin: 0 0 8px; }
h3.sub { font-size: 14px; margin: 18px 0 10px; font-weight: 500; }
.segmented { display: flex; border: 1px solid #e5e6eb; border-radius: 5px; overflow: hidden; }
.segmented button { background: #fff; border: 0; padding: 6px 14px; font-size: 12px; cursor: pointer; color: #4e5969; }
.segmented button.on { background: #f2f3f5; color: #1d2129; font-weight: 500; }
.toolbar { display: flex; gap: 10px; align-items: center; margin: 14px 0; }
.toolbar label { font-size: 12px; color: #4e5969; }
.toolbar input { border: 1px solid #e5e6eb; border-radius: 4px; padding: 6px 8px; width: 90px; margin-left: 4px; }
button.primary { background: #111; color: #fff; border: 0; border-radius: 4px; padding: 7px 16px; cursor: pointer; font-size: 12px; }
button.plain { background: #fff; border: 1px solid #e5e6eb; border-radius: 4px; padding: 7px 14px; cursor: pointer; font-size: 12px; }
.cards { display: flex; gap: 10px; flex-wrap: wrap; }
.card { display: flex; flex-direction: column; align-items: flex-start; border: 1px solid #e5e6eb; background: #fff;
        border-radius: 5px; padding: 10px 14px; cursor: pointer; min-width: 160px; }
.card.active { border-color: #111; background: #f7f7f7; }
.card-name { font-size: 12px; }
.card-count { font-size: 11px; color: #86909c; margin-top: 4px; }
.delivered { font-size: 12px; color: #86909c; margin: 12px 0; }
table { width: 100%; border-collapse: collapse; font-size: 12px; }
th, td { border: 1px solid #e5e6eb; padding: 10px 12px; text-align: left; }
th { background: #fafafa; font-weight: 500; color: #4e5969; }
td.num { text-align: right; font-variant-numeric: tabular-nums; }
td.total { color: #86909c; }
td.empty { text-align: center; color: #86909c; padding: 24px; }
.period { display: inline-block; padding: 2px 12px; border-radius: 3px; font-size: 12px; }
.period.valley { background: #e8f7ee; color: #00a870; }
.period.flat { background: #e6f1fb; color: #185fa5; }
.period.peak { background: #fff7e8; color: #d25f00; }
.period.sharp { background: #ffece8; color: #f53f3f; }
.status { font-size: 12px; }
.status.valid { color: #00a870; }
.status.invalid { color: #86909c; }
.ops button { margin-right: 8px; }
button.link { background: none; border: 0; color: #1677ff; cursor: pointer; font-size: 12px; padding: 0; }
.pager { text-align: right; font-size: 12px; color: #86909c; margin-top: 12px; }
.mono { font-family: ui-monospace, monospace; }
.small { font-size: 11px; }
.notice { color: #00a870; font-size: 13px; }
</style>
