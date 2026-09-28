import { ref } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useTransferStore, normalizeTransfer } from '../stores/transfer'

export function useTransfer() {
  const store = useTransferStore()
  const sending = ref(false)
  const error = ref<string | null>(null)

  async function sendFile(peerId: string, peerName: string, peerAddr: string, filePath: string) {
    sending.value = true
    error.value = null
    try {
      const { SendFile } = await import('../../wailsjs/go/main/App')
      const transferId = await SendFile(peerId, peerName, peerAddr, filePath)

      if (transferId) {
        // The backend emits transferStarted for every task, which carries the
        // authoritative file size. Seeding a placeholder here would duplicate
        // the row once the event arrives, so only add one if it has not.
        if (!store.getTransferById(transferId)) {
          const fileName = filePath.split(/[\\/]/).pop() ?? filePath
          store.addTransfer(
            normalizeTransfer({
              id: transferId,
              peerId,
              peerName,
              peerAddr,
              fileName,
              filePath,
              status: 'pending',
              isSender: true,
              startTime: new Date().toISOString(),
            }),
          )
        }
      }

      return transferId
    } catch (err) {
      error.value = String(err)
      throw err
    } finally {
      sending.value = false
    }
  }

  async function cancelTransfer(transferId: string) {
    error.value = null
    try {
      const { CancelTransfer } = await import('../../wailsjs/go/main/App')
      await CancelTransfer(transferId)
    } catch (err) {
      error.value = String(err)
    }
  }

  // EventsOn returns its own unsubscribe function. The previous implementation
  // probed `window.EventsOn`, which Wails v2 does not create, so this handler
  // never fired.
  function onTransferEvent(callback: (event: unknown) => void) {
    return EventsOn('transferEvent', callback as (...args: unknown[]) => void)
  }

  return {
    store,
    sending,
    error,
    sendFile,
    cancelTransfer,
    onTransferEvent,
  }
}
