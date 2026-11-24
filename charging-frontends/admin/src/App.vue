<script setup>
import { ref, onMounted } from 'vue'
import { loadTenantConfig } from '@opencharge/shared'

const config = ref(null)
onMounted(async () => {
  config.value = await loadTenantConfig({ domain: window.location.hostname })
})
</script>

<template>
  <main>
    <h1>{{ config?.name || 'OpenCharge' }} 运营后台</h1>
    <p>场站 / 设备 / 订单 / 清分管理</p>
  </main>
</template>
