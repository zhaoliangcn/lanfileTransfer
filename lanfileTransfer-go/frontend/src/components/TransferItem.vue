<template>
  <div class="transfer-item">
    <div class="transfer-info">
      <div class="transfer-name">
        <n-ellipsis style="max-width: 200px">
          {{ task.fileName }}
        </n-ellipsis>
        <n-tag
          :type="statusType"
          size="tiny"
          style="margin-left: 8px"
        >
          {{ task.status }}
        </n-tag>
      </div>
      <div class="transfer-meta">
        <span v-if="task.peerName" class="meta-item">
          {{ task.isSender ? 'To:' : 'From:' }} {{ task.peerName }}
        </span>
        <span class="meta-item">{{ store.formatFileSize(task.fileSize) }}</span>
        <span v-if="task.speed > 0" class="meta-item">
          {{ store.formatSpeed(task.speed) }}
        </span>
        <span v-if="eta" class="meta-item eta">
          ETA: {{ eta }}
        </span>
        <span v-if="task.status === 'transferring'" class="meta-item">
          {{ store.formatFileSize(task.bytesTransferred) }} / {{ store.formatFileSize(task.fileSize) }}
        </span>
      </div>
    </div>

    <div class="transfer-progress">
      <n-progress
        :status="task.status === 'failed' ? 'error' : task.status === 'completed' ? 'success' : 'default'"
        type="line"
        :percentage="Math.round(task.progress)"
        :indicator-placement="'inside'"
        :height="20"
        :border-radius="4"
      >
        <template #default>
          {{ Math.round(task.progress) }}%
        </template>
      </n-progress>
    </div>

    <div class="transfer-actions">
      <n-button
        v-if="task.status === 'pending' || task.status === 'transferring'"
        size="tiny"
        quaternary
        type="error"
        @click="$emit('cancel', task.id)"
      >
        Cancel
      </n-button>
      <n-button
        v-if="task.status === 'failed' || task.status === 'cancelled'"
        size="tiny"
        quaternary
        type="primary"
        :loading="resuming"
        @click="handleResume"
      >
        Resume
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { NEllipsis, NTag, NProgress, NButton } from 'naive-ui'
import { useTransferStore } from '../stores/transfer'

const props = defineProps<{
  task: any
}>()

const emit = defineEmits<{
  cancel: [taskId: string]
}>()

const store = useTransferStore()
const resuming = ref(false)

const statusType = computed(() => {
  switch (props.task.status) {
    case 'completed': return 'success'
    case 'failed': return 'error'
    case 'cancelled': return 'default'
    case 'transferring': return 'warning'
    case 'pending': return 'info'
    default: return 'default'
  }
})

const eta = computed(() => {
  if (props.task.speed <= 0 || props.task.status !== 'transferring') return ''
  const remaining = props.task.fileSize - props.task.bytesTransferred
  const seconds = Math.ceil(remaining / props.task.speed)
  if (seconds <= 0) return ''
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) {
    const m = Math.floor(seconds / 60)
    const s = seconds % 60
    return `${m}m ${s}s`
  }
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return `${h}h ${m}m`
})

async function handleResume() {
  resuming.value = true
  try {
    const { ResumeTransfer } = await import('../../wailsjs/go/main/App')
    await ResumeTransfer(props.task.id)
  } catch (err) {
    console.error('Failed to resume transfer:', err)
  } finally {
    resuming.value = false
  }
}
</script>

<style scoped>
.transfer-item {
  width: 100%;
}

.transfer-info {
  margin-bottom: 8px;
}

.transfer-name {
  display: flex;
  align-items: center;
  margin-bottom: 4px;
}

.transfer-meta {
  display: flex;
  gap: 16px;
  font-size: 12px;
  color: #888;
  flex-wrap: wrap;
}

.meta-item {
  white-space: nowrap;
}

.eta {
  color: #1890ff;
  font-weight: 500;
}

.transfer-progress {
  margin-bottom: 4px;
}

.transfer-actions {
  display: flex;
  justify-content: flex-end;
}
</style>