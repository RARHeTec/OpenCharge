<script setup>
import { ref, onMounted } from 'vue'

const props = defineProps({ apiBase: String })
const rows = ref([])
const error = ref('')

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
onMounted(load)
</script>

<template>
  <section>
    <h2>充电订单</h2>
    <p v-if="error" class="error">{{ error }}</p>
    <table>
      <thead>
        <tr><th>交易流水号</th><th>设备/枪</th><th>状态</th><th>桩端金额</th><th>平台金额</th><th>实收</th><th>电价版本</th></tr>
      </thead>
      <tbody>
        <tr v-for="o in rows" :key="o.transactionId">
          <td class="amount">{{ o.transactionId }}</td>
          <td>{{ o.deviceId }} / {{ o.gunNo }}</td>
          <td :class="`state-${o.state}`">{{ o.state }}</td>
          <td class="amount">{{ o.pileAmount }}</td>
          <td class="amount">{{ o.platformAmount }}</td>
          <td class="amount">{{ o.chargeAmount }}</td>
          <td>{{ o.tariffVersionId }}</td>
        </tr>
        <tr v-if="!rows.length"><td colspan="7">暂无订单</td></tr>
      </tbody>
    </table>
  </section>
</template>
