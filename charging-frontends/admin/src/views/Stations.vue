<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'

const props = defineProps({ apiBase: String })
const tenantId = ref('T1')
const stations = ref([])
const devices = ref([])
const liveStates = ref([])
const heartbeats = ref([])
const error = ref('')
const notice = ref('')
const activeStation = ref('')
const streamStatus = ref('connecting') // connecting | live | offline

// new station form
const stForm = ref({ id: '', name: '', timezone: 'Asia/Shanghai' })
// new device form
const dvForm = ref({ deviceId: '', stationId: '', gunCount: 2, defaultUserId: 'U1' })

function req(path, opts) {
  return fetch(`${props.apiBase}${path}`, opts).then(async (res) => {
    const json = await res.json().catch(() => ({}))
    if (!res.ok) throw new Error(json.error || `HTTP ${res.status}`)
    return json
  })
}

async function loadAll() {
  try {
    const q = `?tenantId=${encodeURIComponent(tenantId.value)}`
    const [s, d, c, h] = await Promise.all([
      req(`/api/admin/stations${q}`),
      req(`/api/admin/devices${q}`),
      req('/api/admin/connectors').catch(() => ({ data: [] })),
      req('/api/admin/heartbeats').catch(() => ({ data: [] })),
    ])
    stations.value = s.data || []
    devices.value = d.data || []
    liveStates.value = c.data || []
    heartbeats.value = h.data || []
    error.value = ''
  } catch (e) {
    error.value = `加载失败：${e.message}`
  }
}

async function createStation() {
  notice.value = ''
  try {
    const body = { ...stForm.value, tenantId: tenantId.value }
    await req('/api/admin/stations', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    notice.value = `场站 ${stForm.value.id} 已创建`
    stForm.value = { id: '', name: '', timezone: 'Asia/Shanghai' }
    await loadAll()
  } catch (e) {
    error.value = `创建场站失败：${e.message}`
  }
}

async function createDevice() {
  notice.value = ''
  try {
    const body = { ...dvForm.value, tenantId: tenantId.value }
    await req('/api/admin/devices', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    notice.value = `设备 ${dvForm.value.deviceId} 已登记`
    dvForm.value = { deviceId: '', stationId: '', gunCount: 2, defaultUserId: 'U1' }
    await loadAll()
  } catch (e) {
    error.value = `登记设备失败：${e.message}`
  }
}

// Live state lookup: deviceId:gun -> state
const stateMap = computed(() => {
  const m = {}
  for (const s of liveStates.value) m[`${s.deviceId}:${s.gunNo}`] = s
  return m
})

// Heartbeat counters keyed by device:gun, independent of connector state TTL.
const heartbeatMap = computed(() => {
  const m = {}
  for (const h of heartbeats.value) m[`${h.deviceId}:${h.gunNo}`] = h
  return m
})

function timeAgo(t) {
  if (!t || t.startsWith('0001')) return '—'
  const ms = Date.now() - new Date(t).getTime()
  if (ms < 0) return '刚刚'
  const sec = Math.floor(ms / 1000)
  if (sec < 60) return `${sec}秒前`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}分钟前`
  return `${Math.floor(min / 60)}小时前`
}

const filteredDevices = computed(() =>
  activeStation.value ? devices.value.filter((d) => d.stationId === activeStation.value) : devices.value
)

function gunsOf(d) {
  const out = []
  for (let g = 1; g <= (d.gunCount || 1); g++) {
    out.push({
      gun: g,
      st: stateMap.value[`${d.deviceId}:${g}`],
      hb: heartbeatMap.value[`${d.deviceId}:${g}`],
    })
  }
  return out
}

function statusClass(st) {
  if (!st) return 'unknown'
  return st.status || 'unknown'
}

const statusText = { charging: '充电中', occupied: '占位(已插枪)', idle: '空闲', offline: '离线', faulted: '故障', unknown: '未上报' }

// Live connector status via SSE. The dev proxy injects the API key, so the
// browser stays same-origin; EventSource reconnects automatically on drop.
let eventSource = null
function connectStream() {
  if (typeof EventSource === 'undefined') return
  try {
    eventSource = new EventSource(`${props.apiBase}/api/admin/connector-events`)
    eventSource.onopen = () => { streamStatus.value = 'live' }
    eventSource.onerror = () => { streamStatus.value = 'offline' } // auto-retry by EventSource
    eventSource.addEventListener('status', (ev) => {
      let msg
      try { msg = JSON.parse(ev.data) } catch { return }
      const c = msg.connector
      if (!c || !c.deviceId) return
      const idx = liveStates.value.findIndex(
        (s) => s.deviceId === c.deviceId && s.gunNo === c.gunNo)
      if (idx >= 0) liveStates.value[idx] = c
      else liveStates.value.push(c)
    })
  } catch {
    streamStatus.value = 'offline'
  }
}

onMounted(() => {
  loadAll()
  connectStream()
})
onUnmounted(() => {
  if (eventSource) eventSource.close()
})
</script>

<template>
  <section>
    <h2>场站与设备</h2>
    <p class="hint">场站和设备是计费的依据：账单到达时按设备号查台账，得到所属租户与场站。</p>
    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="notice" class="notice">{{ notice }}</p>

    <div class="toolbar">
      <label>租户 <input v-model="tenantId" @change="loadAll" /></label>
      <span class="stream" :class="streamStatus">{{ streamStatus === 'live' ? '● 实时' : streamStatus === 'connecting' ? '○ 连接中' : '○ 已断开' }}</span>
      <button @click="loadAll">刷新</button>
      <span class="count">{{ stations.length }} 个场站 · {{ devices.length }} 台设备</span>
    </div>

    <div class="cols">
      <div class="panel">
        <h3>场站（{{ stations.length }}）</h3>
        <table class="mini">
          <thead><tr><th>ID</th><th>名称</th><th>时区</th><th>设备</th></tr></thead>
          <tbody>
            <tr v-for="s in stations" :key="s.id"
                :class="{ sel: activeStation === s.id }" @click="activeStation = activeStation === s.id ? '' : s.id">
              <td class="mono">{{ s.id }}</td>
              <td>{{ s.name }}</td>
              <td class="dim">{{ s.timezone }}</td>
              <td>{{ devices.filter((d) => d.stationId === s.id).length }}</td>
            </tr>
            <tr v-if="!stations.length"><td colspan="4" class="dim">暂无场站</td></tr>
          </tbody>
        </table>

        <h4>新建场站</h4>
        <div class="form">
          <input v-model="stForm.id" placeholder="场站 ID，如 S2" />
          <input v-model="stForm.name" placeholder="名称" />
          <input v-model="stForm.timezone" placeholder="Asia/Shanghai" />
          <button :disabled="!stForm.id || !stForm.name" @click="createStation">创建</button>
        </div>
      </div>

      <div class="panel">
        <h3>设备（{{ filteredDevices.length }}）<span v-if="activeStation" class="dim">· 场站 {{ activeStation }}</span></h3>
        <table class="mini">
          <thead><tr><th>设备号</th><th>场站</th><th>协议</th><th>枪 / 心跳</th><th>默认用户</th></tr></thead>
          <tbody>
            <tr v-for="d in filteredDevices" :key="d.deviceId">
              <td class="mono">{{ d.deviceId }}</td>
              <td class="dim">{{ d.stationId }}</td>
              <td class="dim">{{ d.protocol }}</td>
              <td>
                <div v-for="g in gunsOf(d)" :key="g.gun" class="gun-row">
                  <span class="gun" :class="statusClass(g.st)">
                    {{ g.gun }}·{{ statusText[statusClass(g.st)] }}
                  </span>
                  <span class="hb" :title="g.hb ? `最近心跳 ${timeAgo(g.hb.lastHeartbeat)}` : '尚未收到心跳'">
                    ♥ {{ g.hb?.count ?? 0 }}<span v-if="g.hb?.faultCount" class="fault"> / 故障 {{ g.hb.faultCount }}</span>
                  </span>
                </div>
              </td>
              <td class="dim">{{ d.defaultUserId || '—' }}</td>
            </tr>
            <tr v-if="!filteredDevices.length"><td colspan="5" class="dim">暂无设备</td></tr>
          </tbody>
        </table>

        <h4>登记设备</h4>
        <div class="form">
          <input v-model="dvForm.deviceId" placeholder="设备号，如 55031412782305" />
          <select v-model="dvForm.stationId">
            <option value="">选择场站</option>
            <option v-for="s in stations" :key="s.id" :value="s.id">{{ s.id }} · {{ s.name }}</option>
          </select>
          <input v-model.number="dvForm.gunCount" type="number" min="1" placeholder="枪数" class="narrow" />
          <input v-model="dvForm.defaultUserId" placeholder="默认钱包用户" class="narrow" />
          <button :disabled="!dvForm.deviceId || !dvForm.stationId" @click="createDevice">登记</button>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hint { color: #86909c; font-size: 13px; margin-top: -6px; margin-bottom: 14px; }
.toolbar { display: flex; gap: 12px; align-items: center; margin-bottom: 14px; }
.toolbar input { padding: 6px 10px; border: 1px solid #e5e6eb; border-radius: 4px; width: 100px; }
.count { color: #86909c; font-size: 13px; }
.cols { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.panel { background: #fff; border-radius: 8px; padding: 14px; }
.panel h3 { font-size: 14px; margin: 0 0 10px; }
.panel h4 { font-size: 13px; margin: 16px 0 8px; color: #4e5969; }
table.mini { width: 100%; border-collapse: collapse; font-size: 12px; }
table.mini th, table.mini td { text-align: left; padding: 7px 8px; border-bottom: 1px solid #f2f3f5; }
table.mini th { background: #fafafa; color: #4e5969; font-weight: 500; }
table.mini tr.sel { background: #f0f7ff; cursor: pointer; }
table.mini tbody tr:not(.sel):hover { background: #fafbfc; cursor: pointer; }
.form { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.form input, .form select { padding: 6px 9px; border: 1px solid #e5e6eb; border-radius: 4px; font-size: 12px; }
.form input.narrow { width: 80px; }
button { padding: 6px 14px; background: #1677ff; color: #fff; border: none; border-radius: 4px; cursor: pointer; }
button:disabled { opacity: .45; cursor: not-allowed; }
.gun-row { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; }
.hb { font-size: 11px; color: #86909c; font-variant-numeric: tabular-nums; }
.fault { color: #f53f3f; }
.gun { display: inline-block; padding: 2px 7px; border-radius: 10px; font-size: 11px; margin-right: 5px; background: #f2f3f5; color: #4e5969; }
.gun.charging { background: #e8f7ee; color: #00a870; }
.gun.idle { background: #f2f3f5; color: #4e5969; }
.gun.occupied { background: #fff7e8; color: #b35c00; }
.gun.faulted { background: #ffece8; color: #f53f3f; }
.gun.offline, .gun.unknown { background: #fff7e8; color: #d25f00; }
.mono { font-family: ui-monospace, monospace; }
.dim { color: #86909c; }
.notice { color: #00a870; font-size: 13px; }
</style>
