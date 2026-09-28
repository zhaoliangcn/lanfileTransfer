import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export interface Peer {
  id: string
  name: string
  ip: string
  port: number
  online: boolean
  lastSeen: string
  /** True for peers the user added by hand rather than via UDP discovery. */
  manual?: boolean
}

export const useDeviceStore = defineStore('device', () => {
  const peers = ref<Peer[]>([])
  const localPeer = ref<Peer | null>(null)
  const discovering = ref(false)

  const onlinePeers = computed(() =>
    peers.value.filter(p => p.online)
  )

  const offlinePeers = computed(() =>
    peers.value.filter(p => !p.online)
  )

  const onlineCount = computed(() => onlinePeers.value.length)

  function setPeers(newPeers: Peer[]) {
    peers.value = newPeers
  }

  function updatePeer(peer: Peer) {
    const index = peers.value.findIndex(p => p.id === peer.id)
    if (index >= 0) {
      peers.value[index] = peer
    } else {
      peers.value.push(peer)
    }
  }

  function removePeer(peerId: string) {
    const index = peers.value.findIndex(p => p.id === peerId)
    if (index >= 0) {
      peers.value.splice(index, 1)
    }
  }

  function setLocalPeer(peer: Peer) {
    localPeer.value = peer
  }

  function setDiscovering(value: boolean) {
    discovering.value = value
  }

  function getPeerById(id: string): Peer | undefined {
    return peers.value.find(p => p.id === id)
  }

  return {
    peers,
    localPeer,
    discovering,
    onlinePeers,
    offlinePeers,
    onlineCount,
    setPeers,
    updatePeer,
    removePeer,
    setLocalPeer,
    setDiscovering,
    getPeerById,
  }
})