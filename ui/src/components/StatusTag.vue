<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ status: string }>()

const color = computed(() => {
  const s = props.status
  if (s === 'alive' || s === 'active' || s === '成功' || s === 'true') return 'success'
  if (s === 'dead' || s === '失败' || s === 'false') return 'error'
  return 'default'
})
</script>

<template>
  <v-chip :color="color" size="small" variant="tonal" label>
    {{ status }}
  </v-chip>
</template>
