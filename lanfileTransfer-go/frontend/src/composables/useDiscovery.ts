import { ref, onMounted } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useDeviceStore } from '../stores/device'
import type { Peer } from '../stores/device'

export function useDiscovery() {
  const store = useDeviceStore()
  const error = ref<string | null>(null)

  async function refreshPeers() {
    error.value = null
    try {
      store.setDiscovering(true)
      const { GetPeers } = await import('../../wailsjs/go/main/App')
      store.setPeers(await GetPeers())
    } catch (err) {
      error.value = String(err)
    } finally {
      store.setDiscovering(false)
    }
  }

  // EventsOn returns its own unsubscribe function. The previous implementation
  // looked for `window.EventsOn`, which Wails v2 does not create (it injects
  // `window.runtime`), so both helpers below were permanent no-ops.
  function onPeerFound(callback: (peer: Peer) => void) {
    return EventsOn('peerFound', callback as (...args: unknown[]) => void)
  }

  function onPeerLost(callback: (peerId: string) => void) {
    return EventsOn('peerLost', callback as (...args: unknown[]) => void)
  }

  onMounted(() => {
    void refreshPeers()
  })

  return {
    store,
    error,
    refreshPeers,
    onPeerFound,
    onPeerLost,
  }
}
