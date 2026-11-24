<script setup>
import { ref, onMounted } from 'vue'

const props = defineProps({ apiBase: String })
const rows = ref([])
const error = ref('')
const notice = ref('')
const operator = ref('admin')
const busy = ref('')
const decisions = ref({})
const customs = ref({})

async function load() {
  try {
    const res = await fetch(`${props.apiBase}/api/admin/exceptions`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const json = await res.json()
    rows.value = json.data || []
    for (const e of rows.value) {
      if (!decisions.value[e.transactionId]) decisions.value[e.transactionId] = 'platform'
    }
    error.value = ''
  } catch (e) {
    error.value = `加载失败：${e.message}`
  }
}

async function resolve(e) {
  const txn = e.transactionId
  const decision = decisions.value[txn]
  const body = { transactionId: txn, decision, operator: operator.value, note: '' }
  if (decision === 'custom') body.amount = customs.value[txn] || ''
  busy.value = txn
  notice.value = ''
  try {
    const res = await fetch(`${props.apiBase}/api/admin/exceptions/resolve`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    const json = await res.json()
    if (!res.ok) throw new Error(json.error || `HTTP ${res.status}`)
    notice.value = `已处置 ${txn.slice(0, 12)}…：扣款 ${json.chargedAmount}，余额 ${json.walletBalance}`
    error.value = ''
    await load()
  } catch (err) {
    error.value = `处置失败：${err.message}`
  } finally {
    busy.value = ''
  }
}

onMounted(load)
</script>

<template>
  <section>
    <h2>异常单处置</h2>
    <p class="hint">处置会真实扣款并写审计。平台=按平台重算金额，桩端=按设备上报金额，自定义=人工输入。重复处置不会重复扣款。</p>
    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="notice" class="notice">{{ notice }}</p>

    <div class="bar">
      <label>处理人 <input v-model="operator" /></label>
    </div>

    <table>
      <thead>
        <tr>
          <th>流水号</th><th>原因码</th><th>桩端</th><th>平台</th><th>差额</th>
          <th>处置方式</th><th>操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="e in rows" :key="e.id">
          <td class="mono">{{ (e.transactionId || '').slice(0, 12) }}…</td>
          <td><span class="reason">{{ e.reasonCode }}</span></td>
          <td class="amount">{{ e.pileAmount }}</td>
          <td class="amount">{{ e.platformAmount }}</td>
          <td class="amount">{{ e.diff }}</td>
          <td>
            <template v-if="!e.resolved">
              <select v-model="decisions[e.transactionId]">
                <option value="platform">平台 {{ e.platformAmount }}</option>
                <option value="pile">桩端 {{ e.pileAmount }}</option>
                <option value="custom">自定义</option>
              </select>
              <input v-if="decisions[e.transactionId] === 'custom'"
                     v-model="customs[e.transactionId]" class="amt" placeholder="0.75" />
            </template>
            <span v-else class="done">已处置</span>
          </td>
          <td>
            <button v-if="!e.resolved" :disabled="busy === e.transactionId" @click="resolve(e)">
              {{ busy === e.transactionId ? '处理中…' : '确认处置' }}
            </button>
          </td>
        </tr>
        <tr v-if="!rows.length"><td colspan="7">无异常单</td></tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.hint { color: #86909c; font-size: 13px; margin-top: -6px; margin-bottom: 14px; }
.bar { margin-bottom: 12px; }
.bar input { padding: 6px 10px; border: 1px solid #e5e6eb; border-radius: 4px; width: 120px; }
select { padding: 5px 8px; border: 1px solid #e5e6eb; border-radius: 4px; }
.amt { width: 80px; padding: 5px 8px; border: 1px solid #e5e6eb; border-radius: 4px; margin-left: 6px; }
button { padding: 6px 12px; border: 1px solid #1677ff; color: #1677ff; background: #fff; border-radius: 4px; cursor: pointer; }
button:disabled { opacity: .5; }
.reason { background: #ffece8; color: #f53f3f; padding: 2px 8px; border-radius: 4px; font-size: 12px; }
.done { color: #00a870; font-size: 13px; }
.notice { color: #00a870; font-size: 13px; }
.mono { font-family: ui-monospace, monospace; font-size: 12px; }
</style>
