<template>
  <n-card title="Transfers" :bordered="true" class="transfer-panel-card">
    <template #header-extra>
      <n-space size="small">
        <n-button
          v-if="store.failedCount > 0"
          size="tiny"
          type="primary"
          quaternary
          :loading="resumingAll"
          @click="handleResumeAll"
        >
          Resume All ({{ store.failedCount }})
        </n-button>
        <n-button
          v-if="store.completedCount > 0"
          size="tiny"
          quaternary
          @click="clearCompleted"
        >
          Clear Completed
        </n-button>
        <n-tag :type="store.activeCount > 0 ? 'warning' : 'default'" size="small">
          {{ store.activeCount }} active
        </n-tag>
        <n-tag v-if="totalSpeed > 0" type="info" size="small">
          {{ store.formatSpeed(totalSpeed) }}
        </n-tag>
      </n-space>
    </template>

    <div v-if="store.activeCount > 0" class="overall-progress">
      <n-progress
        type="line"
        :percentage="Math.round(overallProgress)"
        :indicator-placement="'inside'"
        :height="16"
        :border-radius="4"
        status="info"
      />
    </div>

    <n-tabs type="line" default-value="all" size="small">
      <n-tab-pane name="all" tab="All">
        <n-empty v-if="store.transfers.length === 0" description="No transfers yet" />
        <n-list v-else>
          <n-list-item v-for="task in store.transfers" :key="task.id">
            <TransferItem :task="task" @cancel="handleCancel" />
          </n-list-item>
        </n-list>
      </n-tab-pane>
      <n-tab-pane name="active" tab="Active">
        <n-empty v-if="store.activeTransfers.length === 0" description="No active transfers" />
        <n-list v-else>
          <n-list-item v-for="task in store.activeTransfers" :key="task.id">
            <TransferItem :task="task" @cancel="handleCancel" />
          </n-list-item>
        </n-list>
      </n-tab-pane>
      <n-tab-pane name="completed" tab="Completed">
        <n-empty v-if="store.completedTransfers.length === 0" description="No completed transfers" />
        <n-list v-else>
          <n-list-item v-for="task in store.completedTransfers" :key="task.id">
            <TransferItem :task="task" @cancel="handleCancel" />
          </n-list-item>
        </n-list>
      </n-tab-pane>
    </n-tabs>
  </n-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  NCard, NButton, NSpace, NTag, NTabs, NTabPane,
  NEmpty, NList, NListItem, NProgress,
} from 'naive-ui'
import { useTransferStore } from '../stores/transfer'
import TransferItem from './TransferItem.vue'

const store = useTransferStore()
const resumingAll = ref(false)

const totalSpeed = computed(() => {
  return store.activeTransfers.reduce((sum, t) => sum + (t.speed || 0), 0)
})

const overallProgress = computed(() => {
  const active = store.activeTransfers
  if (active.length === 0) return 0
  const totalBytes = active.reduce((sum, t) => sum + t.fileSize, 0)
  const totalTransferred = active.reduce((sum, t) => sum + (t.bytesTransferred || 0), 0)
  if (totalBytes === 0) return 0
  return (totalTransferred / totalBytes) * 100
})

async function handleCancel(taskId: string) {
  try {
    const { CancelTransfer } = await import('../../wailsjs/go/main/App')
    await CancelTransfer(taskId)
  } catch (err) {
    console.error('Failed to cancel transfer:', err)
  }
}

async function handleResumeAll() {
  resumingAll.value = true
  try {
    const { ResumeFailedTransfers } = await import('../../wailsjs/go/main/App')
    const count = await ResumeFailedTransfers()
    if (count > 0) {
      console.log(`Resumed ${count} transfers`)
    }
  } catch (err) {
    console.error('Failed to resume transfers:', err)
  } finally {
    resumingAll.value = false
  }
}

function clearCompleted() {
  store.clearCompleted()
}
</script>

<style scoped>
.transfer-panel-card {
  height: 100%;
  overflow: auto;
}

.overall-progress {
  margin-bottom: 8px;
  padding: 0 12px;
}
</style>