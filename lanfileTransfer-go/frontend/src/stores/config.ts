import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useConfigStore = defineStore('config', () => {
  const defaultSavePath = ref('')
  const autoReceive = ref(true)
  const maxConcurrentTransfers = ref(3)
  const chunkSize = ref(65536)
  const listenPort = ref(9876)

  function updateConfig(config: {
    defaultSavePath?: string
    autoReceive?: boolean
    maxConcurrentTransfers?: number
    chunkSize?: number
    listenPort?: number
  }) {
    if (config.defaultSavePath !== undefined) defaultSavePath.value = config.defaultSavePath
    if (config.autoReceive !== undefined) autoReceive.value = config.autoReceive
    if (config.maxConcurrentTransfers !== undefined) maxConcurrentTransfers.value = config.maxConcurrentTransfers
    if (config.chunkSize !== undefined) chunkSize.value = config.chunkSize
    if (config.listenPort !== undefined) listenPort.value = config.listenPort
  }

  return {
    defaultSavePath,
    autoReceive,
    maxConcurrentTransfers,
    chunkSize,
    listenPort,
    updateConfig,
  }
})