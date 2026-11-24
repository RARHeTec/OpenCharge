<script setup>
import { ref, onMounted } from 'vue'

const props = defineProps({ apiBase: String })
const rows = ref([])
const error = ref('')
const operator = ref('admin')

async function load() {
  try {
    const res = await fetch(`${props.apiBase}/api/admin/exceptions`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const json = await res.json()
    rows.value = json.data || []
    error.value = ''
  } catch (e) {
    error.value = `加载失败：${e.message}`
  }
}

async function resolve(id) {
  const body = new URLSearchParams({ id, operator: operator.value })
  const res = await fetch(`${props.apiBase}/api/admin/exceptions/resolve`, {
    method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body
  })
  if (res.ok) load()
  else error.value = '处理失败'
}
onMounted(load)
</script>

<template>
  <section>
    <h2>异常单（不自动扣款）</h2>
    <p v-if="error" class="error">{{ error }}</p>
    <div class="bar">
      <label>处理人 <input v-model="operator" /></label>
    </div>
    <table>
      <thead>
        <tr><th>流水号</th><th>原因码</th><th>桩端金额</th><th>平台金额</th><th>差额</th><th>TraceId</th><th>操作</th></tr>
      </thead>
      <tbody>
        <tr v-for="e in rows" :key="e.id">
          <td class="amount">{{ e.transactionId }}</td>
          <td><span class="reason">{{ e.reasonCode }}</span></td>
          <td class="amount">{{ e.pileAmount }}</td>
          <td class="amount">{{ e.platformAmount }}</td>
          <td class="amount">{{ e.diff }}</td>
          <td class="amount">{{ e.traceId }}</td>
          <td><button v-if="!e.resolved" @click="resolve(e.id)">标记已处理</button><span v-else>已处理</span></td>
        </tr>
        <tr v-if="!rows.length"><td colspan="7">无异常单</td></tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.bar { margin-bottom: 12px; }
.bar input { padding: 6px 10px; border: 1px solid #e5e6eb; border-radius: 4px; }
button { padding: 6px 12px; border: 1px solid #1677ff; color: #1677ff; background: #fff; border-radius: 4px; cursor: pointer; }
.reason { background: #ffece8; color: #f53f3f; padding: 2px 8px; border-radius: 4px; font-size: 12px; }
</style>
