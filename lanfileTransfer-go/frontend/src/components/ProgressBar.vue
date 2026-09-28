<template>
  <n-card title="Progress" size="small" :bordered="true">
    <n-progress
      :type="type"
      :percentage="percentage"
      :height="20"
      :border-radius="4"
      :indicator-placement="'inside'"
      :color="color"
    />
    <div class="progress-label">
      <span>{{ label }}</span>
      <span v-if="showSpeed && speed > 0" class="progress-speed">
        {{ formatSpeed(speed) }}
      </span>
    </div>
  </n-card>
</template>

<script setup lang="ts">
import { NCard, NProgress } from 'naive-ui'

withDefaults(defineProps<{
  percentage: number
  type?: 'line' | 'circle' | 'dashboard'
  label?: string
  speed?: number
  showSpeed?: boolean
  color?: string
}>(), {
  type: 'line',
  label: '',
  speed: 0,
  showSpeed: false,
})

function formatSpeed(bytesPerSec: number): string {
  if (bytesPerSec >= 1073741824) {
    return `${(bytesPerSec / 1073741824).toFixed(2)} GB/s`
  }
  if (bytesPerSec >= 1048576) {
    return `${(bytesPerSec / 1048576).toFixed(2)} MB/s`
  }
  if (bytesPerSec >= 1024) {
    return `${(bytesPerSec / 1024).toFixed(2)} KB/s`
  }
  return `${bytesPerSec.toFixed(0)} B/s`
}
</script>

<style scoped>
.progress-label {
  display: flex;
  justify-content: space-between;
  margin-top: 4px;
  font-size: 12px;
  color: #888;
}

.progress-speed {
  font-weight: 500;
}
</style>