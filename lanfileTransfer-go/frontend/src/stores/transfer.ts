import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export type TransferType = 'file' | 'folder'
export type TransferStatus =
  | 'pending'
  | 'transferring'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'paused'

export interface TransferTask {
  id: string
  peerId: string
  peerName: string
  peerAddr: string
  fileName: string
  filePath: string
  fileSize: number
  bytesTransferred: number
  type: TransferType
  status: TransferStatus
  isSender: boolean
  startTime: string
  endTime: string
  speed: number
  progress: number
  error?: string
  /** Absolute byte offset of the chunk being sent, for resume support. */
  checkpointPath?: string
  relativePath?: string
}

const KNOWN_TYPES: TransferType[] = ['file', 'folder']
const KNOWN_STATUSES: TransferStatus[] = [
  'pending',
  'transferring',
  'completed',
  'failed',
  'cancelled',
  'paused',
]

/**
 * Normalise a task coming from the Go backend. The generated Wails model types
 * these fields as plain `string`, so accepting them unchecked would leak
 * unknown values into the strict union above.
 */
export function normalizeTransfer(raw: Record<string, any>): TransferTask {
  const type = KNOWN_TYPES.includes(raw.type as TransferType)
    ? (raw.type as TransferType)
    : 'file'
  const status = KNOWN_STATUSES.includes(raw.status as TransferStatus)
    ? (raw.status as TransferStatus)
    : 'pending'

  return {
    id: String(raw.id ?? ''),
    peerId: raw.peerId ?? '',
    peerName: raw.peerName ?? '',
    peerAddr: raw.peerAddr ?? '',
    fileName: raw.fileName ?? '',
    filePath: raw.filePath ?? '',
    fileSize: Number(raw.fileSize ?? 0),
    bytesTransferred: Number(raw.bytesTransferred ?? 0),
    type,
    status,
    isSender: Boolean(raw.isSender),
    startTime: raw.startTime ?? '',
    endTime: raw.endTime ?? '',
    speed: Number(raw.speed ?? 0),
    progress: Number(raw.progress ?? 0),
    error: raw.error,
    checkpointPath: raw.checkpointPath,
    relativePath: raw.relativePath,
  }
}

export const useTransferStore = defineStore('transfer', () => {
  const transfers = ref<TransferTask[]>([])

  const activeTransfers = computed(() =>
    transfers.value.filter(
      t => t.status === 'pending' || t.status === 'transferring'
    )
  )

  const completedTransfers = computed(() =>
    transfers.value.filter(t => t.status === 'completed')
  )

  const failedTransfers = computed(() =>
    transfers.value.filter(t => t.status === 'failed')
  )

  const activeCount = computed(() => activeTransfers.value.length)
  const completedCount = computed(() => completedTransfers.value.length)
  const failedCount = computed(() => failedTransfers.value.length)

  /** Insert, or replace an existing entry with the same id. */
  function upsertTransfer(task: TransferTask) {
    const index = transfers.value.findIndex(t => t.id === task.id)
    if (index >= 0) {
      transfers.value[index] = { ...transfers.value[index], ...task }
    } else {
      transfers.value.unshift(task)
    }
  }

  function addTransfer(task: TransferTask) {
    upsertTransfer(task)
  }

  function updateTransfer(id: string, updates: Partial<TransferTask>) {
    const index = transfers.value.findIndex(t => t.id === id)
    if (index >= 0) {
      transfers.value[index] = { ...transfers.value[index], ...updates }
    }
  }

  function removeTransfer(id: string) {
    const index = transfers.value.findIndex(t => t.id === id)
    if (index >= 0) {
      transfers.value.splice(index, 1)
    }
  }

  function clearCompleted() {
    transfers.value = transfers.value.filter(
      t => t.status !== 'completed'
    )
  }

  function getTransferById(id: string): TransferTask | undefined {
    return transfers.value.find(t => t.id === id)
  }

  function formatFileSize(bytes: number): string {
    if (bytes >= 1073741824) {
      return `${(bytes / 1073741824).toFixed(2)} GB`
    }
    if (bytes >= 1048576) {
      return `${(bytes / 1048576).toFixed(2)} MB`
    }
    if (bytes >= 1024) {
      return `${(bytes / 1024).toFixed(2)} KB`
    }
    return `${bytes} B`
  }

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

  return {
    transfers,
    activeTransfers,
    completedTransfers,
    failedTransfers,
    activeCount,
    completedCount,
    failedCount,
    addTransfer,
    upsertTransfer,
    updateTransfer,
    removeTransfer,
    clearCompleted,
    getTransferById,
    formatFileSize,
    formatSpeed,
  }
})