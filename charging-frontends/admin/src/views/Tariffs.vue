<script setup>
import { ref, computed, onMounted } from 'vue'

const props = defineProps({ apiBase: String })
const tenantId = ref('T1')
const stationId = ref('S1')

const versions = ref([])
const error = ref('')
const notice = ref('')
const saving = ref(false)

// 48 half-hour slots, each with a 5-decimal price string.
const prices = ref(Array.from({ length: 48 }, () => '0.60000'))
const effective = ref(defaultEffective())

function defaultEffective() {
  const d = new Date()
  d.setSeconds(0, 0)
  const pad = (n) => String(n).padStart(2, '0')
  const off = -d.getTimezoneOffset()
  const sign = off >= 0 ? '+' : '-'
  const hh = pad(Math.floor(Math.abs(off) / 60))
  const mm = pad(Math.abs(off) % 60)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:00${sign}${hh}:${mm}`
}

function slotTime(slot) {
  const h = Math.floor(slot / 2)
  const m = slot % 2 === 0 ? '00' : '30'
  return `${String(h).padStart(2, '0')}:${m}`
}

// Period presets: 尖峰平谷 ranges (configurable per tenant in a real system).
const presets = [
  { name: '谷', from: 0, to: 15, price: '0.30000' },
  { name: '平', from: 16, to: 23, price: '0.60000' },
  { name: '峰', from: 24, to: 39, price: '0.90000' },
  { name: '尖', from: 40, to: 47, price: '1.20000' },
]

function applyPresets() {
  const next = [...prices.value]
  for (const p of presets) {
    for (let i = p.from; i <= p.to && i < 48; i++) next[i] = p.price
  }
  prices.value = next
}

const priceRe = /^\d+(\.\d{1,5})?$/
const invalidSlots = computed(() => {
  const bad = []
  prices.value.forEach((v, i) => {
    if (!priceRe.test(v.trim())) bad.push(i)
  })
  return bad
})

const canSave = computed(() => invalidSlots.value.length === 0 && tenantId.value && stationId.value)

async function loadVersions() {
  if (!tenantId.value) return
  try {
    const url = `${props.apiBase}/api/admin/tariffs?tenantId=${encodeURIComponent(tenantId.value)}&stationId=${encodeURIComponent(stationId.value)}`
    const res = await fetch(url)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const json = await res.json()
    versions.value = json.data || []
    error.value = ''
  } catch (e) {
    error.value = `加载失败：${e.message}`
  }
}

async function save() {
  if (!canSave.value) return
  saving.value = true
  notice.value = ''
  try {
    const slots = prices.value.map((p, i) => ({ slot: i, price: p.trim() }))
    const res = await fetch(`${props.apiBase}/api/admin/tariffs`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        tenantId: tenantId.value,
        stationId: stationId.value,
        effective: effective.value,
        slots,
      }),
    })
    const json = await res.json()
    if (!res.ok) throw new Error(json.error || `HTTP ${res.status}`)
    notice.value = `已发布版本 ${json.id}，生效时间 ${json.effective}`
    await loadVersions()
  } catch (e) {
    error.value = `发布失败：${e.message}`
  } finally {
    saving.value = false
  }
}

function loadIntoEditor(v) {
  if (!v.slots) return
  const next = Array.from({ length: 48 }, () => '0.00000')
  for (const s of v.slots) next[s.slot] = s.price
  prices.value = next
  effective.value = v.effective
  notice.value = `已载入版本 ${v.id} 的价格，修改后发布将生成新版本`
}

onMounted(loadVersions)
</script>

<template>
  <section>
    <h2>电价版本管理</h2>
    <p class="hint">电价版本是不可变快照：修改后发布会生成新版本，历史订单仍按原版本计价。</p>

    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="notice" class="notice">{{ notice }}</p>

    <div class="toolbar">
      <label>租户 <input v-model="tenantId" @change="loadVersions" /></label>
      <label>场站 <input v-model="stationId" @change="loadVersions" /></label>
      <label>生效时间 <input v-model="effective" class="wide" /></label>
      <button @click="loadVersions">刷新</button>
    </div>

    <div class="editor">
      <div class="editor-head">
        <strong>48 时段价格（元/kWh，最多 5 位小数）</strong>
        <div class="presets">
          <button v-for="p in presets" :key="p.name" class="ghost"
                  @click="prices = prices.map((v, i) => (i >= p.from && i <= p.to ? p.price : v))">
            批量设 {{ p.name }} {{ p.price }}
          </button>
          <button class="ghost" @click="applyPresets">应用默认尖峰平谷</button>
        </div>
      </div>

      <div class="grid">
        <div v-for="(price, i) in prices" :key="i" class="cell" :class="{ bad: invalidSlots.includes(i) }">
          <span class="time">{{ slotTime(i) }}</span>
          <input v-model="prices[i]" class="price" placeholder="0.60000" />
        </div>
      </div>

      <div class="footer">
        <span v-if="invalidSlots.length" class="error">
          {{ invalidSlots.length }} 个时段价格格式不合法（需 0 或最多 5 位小数）
        </span>
        <button :disabled="!canSave || saving" class="primary" @click="save">
          {{ saving ? '发布中…' : '发布新版本' }}
        </button>
      </div>
    </div>

    <h3>历史版本</h3>
    <table>
      <thead><tr><th>版本 ID</th><th>生效时间</th><th>时段数</th><th>操作</th></tr></thead>
      <tbody>
        <tr v-for="v in versions" :key="v.id">
          <td class="mono">{{ v.id }}</td>
          <td>{{ v.effective }}</td>
          <td>{{ v.slots?.length ?? 0 }}</td>
          <td><button class="ghost" @click="loadIntoEditor(v)">载入编辑</button></td>
        </tr>
        <tr v-if="!versions.length"><td colspan="4">暂无版本</td></tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.hint { color: #86909c; font-size: 13px; margin-top: -6px; }
.toolbar { display: flex; gap: 14px; align-items: flex-end; flex-wrap: wrap; margin-bottom: 16px; }
.toolbar label { display: flex; flex-direction: column; font-size: 12px; color: #4e5969; gap: 4px; }
.toolbar input { padding: 6px 10px; border: 1px solid #e5e6eb; border-radius: 4px; width: 110px; }
.toolbar input.wide { width: 230px; }
.editor { background: #fff; border-radius: 8px; padding: 16px; margin-bottom: 24px; }
.editor-head { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px; margin-bottom: 12px; }
.presets { display: flex; gap: 8px; flex-wrap: wrap; }
.grid { display: grid; grid-template-columns: repeat(6, 1fr); gap: 8px; }
.cell { display: flex; align-items: center; gap: 6px; padding: 6px; border: 1px solid #f0f1f2; border-radius: 6px; }
.cell.bad { border-color: #f53f3f; background: #fff5f5; }
.time { font-size: 12px; color: #86909c; font-variant-numeric: tabular-nums; width: 38px; }
.price { width: 100%; border: 1px solid #e5e6eb; border-radius: 4px; padding: 4px 6px; font-family: ui-monospace, monospace; font-size: 12px; }
.footer { display: flex; justify-content: flex-end; align-items: center; gap: 16px; margin-top: 16px; }
button { padding: 6px 14px; border-radius: 4px; border: 1px solid #e5e6eb; background: #fff; cursor: pointer; font-size: 13px; }
button.ghost { color: #1677ff; border-color: #bedaff; }
button.primary { background: #1677ff; color: #fff; border-color: #1677ff; }
button:disabled { opacity: .5; cursor: not-allowed; }
.notice { color: #00a870; font-size: 13px; }
.mono { font-family: ui-monospace, monospace; font-size: 12px; }
</style>
