<script setup>
import { ref, computed, onMounted } from 'vue'

const props = defineProps({
  apiBase: String, tenantId: String, stationId: String,
  deviceTypes: { type: Array, default: () => [] }, initialPlan: Object,
})
const emit = defineEmits(['close', 'saved'])

const effectiveNow = ref(true)
const effectiveAt = ref('')
const selectedTypes = ref(props.deviceTypes.map((d) => d.name))
const operator = ref('admin')
const source = ref('manual')
const error = ref('')
const saving = ref(false)

// Segments use minutes from midnight. Dragging changes boundaries while
// keeping the chain contiguous from 00:00 to 24:00.
const segments = ref([])

function fmtMin(m) {
  return `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`
}

function initSegments() {
  if (props.initialPlan?.segments?.length) {
    segments.value = props.initialPlan.segments.map((s) => ({ ...s }))
    return
  }
  segments.value = [
    { seq: 1, startMin: 0, endMin: 420, period: 'valley', elecPrice: '0.60000', servicePrice: '0.18000' },
    { seq: 2, startMin: 420, endMin: 1440, period: 'flat', elecPrice: '0.88000', servicePrice: '0.15000' },
  ]
}

onMounted(initSegments)

const total = (s) => ((Number(s.elecPrice) || 0) + (Number(s.servicePrice) || 0)).toFixed(4)
const periodLabel = (p) => ({ sharp: '尖', peak: '峰', flat: '平', valley: '谷' }[p] || p)
const periodClass = (p) => ({ sharp: 'sharp', peak: 'peak', flat: 'flat', valley: 'valley' }[p] || 'flat')

// --- drag handling on the timeline ---
const track = ref(null)
const dragging = ref(-1) // index of the boundary being dragged

function onDown(e, index) {
  dragging.value = index
  e.preventDefault()
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}
function onMove(e) {
  if (dragging.value < 0 || !track.value) return
  const rect = track.value.getBoundingClientRect()
  const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
  // snap to 30 minutes, keep 30-minute minimum for both neighbours
  let min = Math.round((ratio * 1440) / 30) * 30
  const i = dragging.value
  const prevEnd = i === 0 ? 0 : segments.value[i - 1].endMin
  const nextEnd = segments.value[i].endMin
  min = Math.max(prevEnd + 30, Math.min(min, nextEnd - 30))
  segments.value[i].endMin = min
  if (i + 1 < segments.value.length) segments.value[i + 1].startMin = min
}
function onUp() {
  dragging.value = -1
  window.removeEventListener('mousemove', onMove)
  window.removeEventListener('mouseup', onUp)
}

const bandColors = {
  sharp: '#f53f3f', peak: '#fa8c16', flat: '#4a90e2', valley: '#52c41a',
}
function bandStyle(seg) {
  return {
    left: `${(seg.startMin / 1440) * 100}%`,
    width: `${((seg.endMin - seg.startMin) / 1440) * 100}%`,
    background: bandColors[seg.period] || bandColors.flat,
  }
}

async function save() {
  error.value = ''
  if (!selectedTypes.value.length) {
    error.value = '请至少选择一个设备类型'
    return
  }
  if (!effectiveNow.value && !effectiveAt.value) {
    error.value = '定时生效请选择时间'
    return
  }
  saving.value = true
  try {
    const body = {
      tenantId: props.tenantId,
      stationId: props.stationId,
      deviceTypes: selectedTypes.value,
      effectiveNow: effectiveNow.value,
      effectiveAt: effectiveNow.value ? '' : new Date(effectiveAt.value).toISOString(),
      source: source.value,
      operator: operator.value,
      segments: segments.value.map((s, i) => ({
        seq: i + 1, startMin: s.startMin, endMin: s.endMin,
        period: s.period,
        elecPrice: String(s.elecPrice), servicePrice: String(s.servicePrice),
      })),
    }
    const res = await fetch(`${props.apiBase}/api/admin/price-plans`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    const json = await res.json().catch(() => ({}))
    if (!res.ok) throw new Error(json.error || `HTTP ${res.status}`)
    emit('saved', `定价已保存：${selectedTypes.value.length} 个设备类型，${segments.value.length} 个时段`)
  } catch (e) {
    error.value = `保存失败：${e.message}`
  } finally {
    saving.value = false
  }
}

// Quick actions kept explicit: operators choose the source, no silent overwrite.
function copyFromOther() {
  error.value = '请从历史定价中选择一条记录后使用「详情」复制（即将支持）'
}
function importIndustrial() {
  error.value = '工商业电价导入需配置外部数据源（即将支持）'
}
</script>

<template>
  <div class="mask">
    <div class="dialog">
      <div class="title">
        <span>调整定价</span>
        <button class="x" @click="emit('close')">×</button>
      </div>
      <div class="body">
        <p v-if="error" class="error">{{ error }}</p>

        <div class="row">
          <label class="lbl required">生效时间：</label>
          <div class="opts">
            <label><input type="radio" v-model="effectiveNow" :value="true" /> 即时生效</label>
            <label><input type="radio" v-model="effectiveNow" :value="false" /> 定时生效</label>
            <input v-if="!effectiveNow" type="datetime-local" v-model="effectiveAt" class="dt" />
          </div>
        </div>

        <div class="row">
          <label class="lbl required">设备类型（桩数）：</label>
          <div class="opts">
            <label v-for="t in deviceTypes" :key="t.name" class="chk">
              <input type="checkbox" :value="t.name" v-model="selectedTypes" /> {{ t.name }} ({{ t.count }})
            </label>
          </div>
        </div>

        <div class="row">
          <label class="lbl required">设置时间：</label>
          <span class="hint-text">拖动设置时间</span>
        </div>
        <div class="timeline-wrap">
          <div class="hours">
            <span v-for="h in 24" :key="h">{{ h - 1 }}</span>
          </div>
          <div class="timeline" ref="track">
            <div v-for="seg in segments" :key="seg.seq" class="band"
                 :style="bandStyle(seg)" :title="`${fmtMin(seg.startMin)}-${fmtMin(seg.endMin)} ${periodLabel(seg.period)}`">
              <span class="band-label">{{ fmtMin(seg.startMin) }}–{{ fmtMin(seg.endMin) }}</span>
            </div>
            <div v-for="(seg, i) in segments" :key="'h' + i" class="handle"
                 :style="{ left: `${(seg.endMin / 1440) * 100}%` }"
                 @mousedown="onDown($event, i)" v-show="i < segments.length - 1"></div>
          </div>
        </div>

        <div class="row price-head">
          <label class="lbl required">设置价格：</label>
          <div class="quick">
            <button class="link" @click="importIndustrial">↓ 引用代理工商业电价</button>
            <button class="link" @click="copyFromOther">⧉ 复制其他场站价格</button>
          </div>
        </div>
        <table class="price-table">
          <thead>
            <tr><th>序号</th><th>时间</th><th>时段</th><th>电费 (元/kWh)</th><th>服务费 (元/kWh)</th><th>总单价 (元/kWh)</th></tr>
          </thead>
          <tbody>
            <tr v-for="(seg, i) in segments" :key="seg.seq">
              <td>{{ i + 1 }}</td>
              <td class="mono">{{ fmtMin(seg.startMin) }}~{{ fmtMin(seg.endMin) }}</td>
              <td>
                <select v-model="seg.period" class="period-sel" :class="periodClass(seg.period)">
                  <option value="valley">谷</option><option value="flat">平</option>
                  <option value="peak">峰</option><option value="sharp">尖</option>
                </select>
              </td>
              <td><input v-model="seg.elecPrice" class="price-in" /></td>
              <td><input v-model="seg.servicePrice" class="price-in" /></td>
              <td class="total">{{ total(seg) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="footer">
        <button class="cancel" @click="emit('close')">取消</button>
        <button class="ok" :disabled="saving" @click="save">{{ saving ? '保存中…' : '确定' }}</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mask { position: absolute; inset: 0; background: rgba(0,0,0,.28); z-index: 50; display: flex; align-items: flex-start; justify-content: center; }
.dialog { background: #fff; border-radius: 6px; width: 1040px; max-width: 96%; margin-top: 24px; box-shadow: 0 8px 24px rgba(0,0,0,.12); }
.title { display: flex; justify-content: space-between; align-items: center; padding: 14px 20px; border-bottom: 1px solid #f0f0f0; font-size: 15px; }
.x { background: none; border: 0; font-size: 20px; cursor: pointer; color: #86909c; }
.body { padding: 18px 20px; max-height: 70vh; overflow: auto; }
.row { display: flex; align-items: flex-start; margin-bottom: 16px; }
.lbl { font-size: 13px; width: 118px; flex-shrink: 0; padding-top: 4px; }
.required::before { content: '*'; color: #f53f3f; margin-right: 4px; }
.opts { display: flex; gap: 18px; flex-wrap: wrap; align-items: center; font-size: 13px; }
.opts label { cursor: pointer; }
.chk { margin-right: 8px; }
.dt { border: 1px solid #e5e6eb; border-radius: 4px; padding: 5px 8px; font-size: 12px; }
.hint-text { font-size: 12px; color: #86909c; }
.timeline-wrap { margin: 0 0 18px 118px; }
.hours { display: flex; justify-content: space-between; font-size: 11px; color: #86909c; margin-bottom: 4px; padding: 0 2px; }
.hours span { width: 12px; text-align: center; }
.timeline { position: relative; height: 46px; border: 1px solid #e5e6eb; background: repeating-linear-gradient(90deg, transparent, transparent calc(100%/48 - 1px), #eef0f2 calc(100%/48 - 1px), #eef0f2 calc(100%/48)); }
.band { position: absolute; top: 4px; height: 36px; opacity: .8; display: flex; align-items: center; justify-content: center; color: #fff; font-size: 11px; }
.band-label { white-space: nowrap; }
.handle { position: absolute; top: -2px; width: 6px; height: 50px; background: #111; cursor: ew-resize; margin-left: -3px; z-index: 2; border-radius: 2px; }
.price-head { align-items: center; margin-bottom: 8px; }
.quick { flex: 1; display: flex; gap: 16px; }
button.link { background: none; border: 0; color: #1677ff; cursor: pointer; font-size: 12px; padding: 0; }
.price-table { width: calc(100% - 118px); margin-left: 118px; border-collapse: collapse; font-size: 12px; }
.price-table th, .price-table td { border: 1px solid #e5e6eb; padding: 8px 10px; text-align: left; }
.price-table th { background: #fafafa; font-weight: 500; }
.price-in { width: 100%; border: 1px solid #e5e6eb; border-radius: 3px; padding: 6px 8px; font-size: 12px; box-sizing: border-box; }
.price-table td.total { background: #f7f8fa; color: #4e5969; font-variant-numeric: tabular-nums; }
.period-sel { border: 1px solid #e5e6eb; border-radius: 3px; padding: 5px 8px; font-size: 12px; }
.period-sel.valley { color: #00a870; }
.period-sel.flat { color: #185fa5; }
.period-sel.peak { color: #d25f00; }
.period-sel.sharp { color: #f53f3f; }
.footer { display: flex; justify-content: flex-end; gap: 10px; padding: 12px 20px; border-top: 1px solid #f0f0f0; }
.footer button { padding: 7px 20px; border-radius: 4px; cursor: pointer; font-size: 13px; }
.cancel { background: #fff; border: 1px solid #e5e6eb; }
.ok { background: #111; color: #fff; border: 0; }
.ok:disabled { opacity: .5; }
.error { color: #f53f3f; font-size: 12px; background: #fff2f0; padding: 8px 10px; border-radius: 4px; }
.mono { font-family: ui-monospace, monospace; }
</style>
