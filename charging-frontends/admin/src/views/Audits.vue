<script setup>
import { ref, onMounted } from 'vue'

const props = defineProps({ apiBase: String })
const rows = ref([])
async function load() {
  const res = await fetch(`${props.apiBase}/api/admin/audits`)
  const json = await res.json()
  rows.value = json.data || []
}
onMounted(load)
</script>

<template>
  <section>
    <h2>容差自动结算审计</h2>
    <table>
      <thead><tr><th>流水号</th><th>规则</th><th>桩端</th><th>平台</th><th>差额</th><th>实收</th><th>TraceId</th></tr></thead>
      <tbody>
        <tr v-for="a in rows" :key="a.id">
          <td class="amount">{{ a.transactionId }}</td>
          <td>{{ a.rule }}</td>
          <td class="amount">{{ a.pileAmount }}</td>
          <td class="amount">{{ a.platformAmount }}</td>
          <td class="amount">{{ a.diff }}</td>
          <td class="amount">{{ a.chargeAmount }}</td>
          <td class="amount">{{ a.traceId }}</td>
        </tr>
        <tr v-if="!rows.length"><td colspan="7">暂无审计记录</td></tr>
      </tbody>
    </table>
  </section>
</template>
