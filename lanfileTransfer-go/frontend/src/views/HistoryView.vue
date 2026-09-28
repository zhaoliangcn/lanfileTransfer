<template>
  <div class="history-view">
    <n-card title="Transfer History">
      <template #header-extra>
        <n-space align="center" size="small">
          <n-button
            size="tiny"
            quaternary
            @click="refreshTransfers"
          >
            Refresh
          </n-button>
          <n-button
            size="tiny"
            quaternary
            type="warning"
            :disabled="store.completedCount === 0"
            @click="handleClearCompleted"
          >
            Clear Completed
          </n-button>
        </n-space>
      </template>

      <n-space vertical>
        <n-space>
          <n-input
            v-model:value="searchQuery"
            placeholder="Search by file name or peer..."
            clearable
            style="width: 280px"
            size="small"
          />
          <n-select
            v-model:value="statusFilter"
            :options="filterOptions"
            style="width: 140px"
            size="small"
            clearable
            placeholder="All Status"
          />
          <n-select
            v-model:value="directionFilter"
            :options="directionOptions"
            style="width: 120px"
            size="small"
            clearable
            placeholder="All"
          />
          <n-button
            size="small"
            quaternary
            @click="resetFilters"
          >
            Reset
          </n-button>
        </n-space>

        <n-space v-if="store.transfers.length > 0" size="small">
          <n-tag size="small" type="info">Total: {{ filteredTransfers.length }}</n-tag>
          <n-tag size="small" type="success">Completed: {{ store.completedCount }}</n-tag>
          <n-tag size="small" type="error">Failed: {{ store.failedCount }}</n-tag>
        </n-space>

        <n-empty v-if="filteredTransfers.length === 0" description="No transfer history" />

        <n-data-table
          v-else
          :columns="columns"
          :data="filteredTransfers"
          :pagination="pagination"
          :bordered="false"
          :single-line="false"
          :row-key="(row: any) => row.id"
          :loading="loading"
          size="small"
        />
      </n-space>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, h } from 'vue'
import {
  NCard, NEmpty, NDataTable, NTag, NProgress, NButton,
  NSpace, NInput, NSelect,
} from 'naive-ui'
import { useTransferStore, normalizeTransfer } from '../stores/transfer'
import { notifyError } from '../composables/useNotification'

const store = useTransferStore()
const loading = ref(false)
const searchQuery = ref('')
const statusFilter = ref<string | null>('')
const directionFilter = ref<string | null>('')

const filterOptions = [
  { label: 'All Status', value: '' },
  { label: 'Completed', value: 'completed' },
  { label: 'Transferring', value: 'transferring' },
  { label: 'Failed', value: 'failed' },
  { label: 'Cancelled', value: 'cancelled' },
  { label: 'Pending', value: 'pending' },
]

const directionOptions = [
  { label: 'All', value: '' },
  { label: 'Sent', value: 'true' },
  { label: 'Received', value: 'false' },
]

const filteredTransfers = computed(() => {
  let list = store.transfers

  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase()
    list = list.filter(t =>
      t.fileName.toLowerCase().includes(q) ||
      (t.peerName && t.peerName.toLowerCase().includes(q))
    )
  }

  if (statusFilter.value) {
    list = list.filter(t => t.status === statusFilter.value)
  }

  if (directionFilter.value) {
    const isSender = directionFilter.value === 'true'
    list = list.filter(t => t.isSender === isSender)
  }

  return list
})

const pagination = { pageSize: 20, pageSizes: [10, 20, 50] }

const statusConfig: Record<string, { type: 'success' | 'error' | 'warning' | 'info' | 'default', label: string }> = {
  completed: { type: 'success', label: 'Completed' },
  failed: { type: 'error', label: 'Failed' },
  cancelled: { type: 'default', label: 'Cancelled' },
  transferring: { type: 'warning', label: 'Transferring' },
  pending: { type: 'info', label: 'Pending' },
  paused: { type: 'default', label: 'Paused' },
}

const columns = [
  {
    title: 'File',
    key: 'fileName',
    ellipsis: { tooltip: true },
    width: 200,
    sorter: (a: any, b: any) => a.fileName.localeCompare(b.fileName),
  },
  {
    title: 'Peer',
    key: 'peerName',
    width: 130,
    render: (row: any) => row.peerName || '-',
  },
  {
    title: 'Size',
    key: 'fileSize',
    width: 90,
    sorter: (a: any, b: any) => a.fileSize - b.fileSize,
    render: (row: any) => store.formatFileSize(row.fileSize),
  },
  {
    title: 'Progress',
    key: 'progress',
    width: 170,
    render: (row: any) => h(NProgress, {
      status: row.status === 'failed' ? 'error' : row.status === 'completed' ? 'success' : 'default',
      type: 'line',
      percentage: Math.round(row.progress),
      height: 14,
      indicatorPlacement: 'inside',
      railColor: '#e8e8e8',
    }),
  },
  {
    title: 'Status',
    key: 'status',
    width: 110,
    sorter: (a: any, b: any) => a.status.localeCompare(b.status),
    render: (row: any) => {
      const cfg = statusConfig[row.status] || { type: 'default', label: row.status }
      return h(NTag, { type: cfg.type, size: 'small' }, { default: () => cfg.label })
    },
  },
  {
    title: 'Direction',
    key: 'isSender',
    width: 90,
    render: (row: any) => {
      if (row.isSender) return h(NTag, { type: 'primary', size: 'small' }, { default: () => 'Sent' })
      return h(NTag, { type: 'info', size: 'small' }, { default: () => 'Received' })
    },
  },
  {
    title: 'Speed',
    key: 'speed',
    width: 110,
    render: (row: any) => row.speed > 0 ? store.formatSpeed(row.speed) : '-',
  },
  {
    title: 'Start Time',
    key: 'startTime',
    width: 160,
    ellipsis: { tooltip: true },
    render: (row: any) => row.startTime ? formatTime(row.startTime) : '-',
  },
  {
    title: 'Error',
    key: 'error',
    width: 160,
    ellipsis: { tooltip: true },
    render: (row: any) => {
      if (!row.error) return '-'
      return h(NTag, { type: 'error', size: 'tiny' }, { default: () => row.error })
    },
  },
  {
    title: 'Actions',
    key: 'actions',
    width: 80,
    render: (row: any) => {
      if (row.status === 'pending' || row.status === 'transferring') {
        return h(NButton, {
          size: 'tiny',
          type: 'error',
          quaternary: true,
          onClick: () => handleCancel(row.id),
        }, { default: () => 'Cancel' })
      }
      if (row.status === 'failed' || row.status === 'cancelled') {
        return h(NButton, {
          size: 'tiny',
          type: 'primary',
          quaternary: true,
          onClick: () => handleResume(row.id),
        }, { default: () => 'Resume' })
      }
      return null
    },
  },
]

function formatTime(ts: string): string {
  try {
    const d = new Date(ts)
    return d.toLocaleString()
  } catch {
    return ts
  }
}

function resetFilters() {
  searchQuery.value = ''
  statusFilter.value = ''
  directionFilter.value = ''
}

async function refreshTransfers() {
  loading.value = true
  try {
    // Merge instead of append: addTransfer() unshifts unconditionally, so
    // every click on Refresh used to double every row in the table.
    const { GetTransferHistory, GetAllTasks } = await import('../../wailsjs/go/main/App')
    const [session, persisted] = await Promise.all([GetAllTasks(), GetTransferHistory()])

    for (const raw of [...(persisted ?? []), ...(session ?? [])]) {
      store.upsertTransfer(normalizeTransfer(raw as unknown as Record<string, any>))
    }
  } catch (err) {
    notifyError('Failed to refresh transfers', String(err))
  } finally {
    loading.value = false
  }
}

async function handleClearCompleted() {
  store.clearCompleted()
}

async function handleCancel(taskId: string) {
  try {
    const { CancelTransfer } = await import('../../wailsjs/go/main/App')
    await CancelTransfer(taskId)
  } catch (err) {
    console.error('Failed to cancel transfer:', err)
  }
}

async function handleResume(taskId: string) {
  try {
    const { ResumeTransfer } = await import('../../wailsjs/go/main/App')
    await ResumeTransfer(taskId)
  } catch (err) {
    console.error('Failed to resume transfer:', err)
  }
}
</script>

<style scoped>
.history-view {
  padding: 16px;
}
</style>