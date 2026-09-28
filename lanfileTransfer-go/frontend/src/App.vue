<template>
  <n-message-provider>
    <n-dialog-provider>
      <n-notification-provider>
        <div class="app-container">
          <n-layout position="absolute">
            <n-layout-header>
              <div class="header-content">
                <div class="header-title">
                  <n-h3 style="margin: 0; cursor: pointer" @click="activeTab = 'home'">LanFileTransfer</n-h3>
                  <n-tag :type="onlineCount > 0 ? 'success' : 'default'" size="small">
                    {{ onlineCount }} devices online
                  </n-tag>
                  <n-tag v-if="transferStore.activeCount > 0" type="warning" size="small">
                    {{ transferStore.activeCount }} active
                  </n-tag>
                </div>
                <div class="header-nav">
                  <n-menu
                    v-model:value="activeTab"
                    mode="horizontal"
                    :options="menuOptions"
                    :indent="0"
                  />
                </div>
              </div>
            </n-layout-header>
            <n-layout-content class="layout-content">
              <div v-if="activeTab === 'home'" class="main-content">
                <DeviceList
                  :peers="peers"
                  :loading="discovering"
                  @send="handleSendToPeer"
                  @refresh="refreshPeers"
                  @peer-added="onPeerAdded"
                  @peer-removed="onPeerRemoved"
                />
                <TransferPanel />
              </div>
              <keep-alive>
                <ChatView v-if="activeTab === 'chat'" />
              </keep-alive>
              <HistoryView v-if="activeTab === 'history'" />
              <SettingsView v-if="activeTab === 'settings'" />
            </n-layout-content>
          </n-layout>
        </div>
      </n-notification-provider>
    </n-dialog-provider>
  </n-message-provider>
</template>

<script setup lang="ts">
import { ref, computed, h, onMounted, onUnmounted } from 'vue'
import {
  NMessageProvider,
  NDialogProvider,
  NNotificationProvider,
  NLayout,
  NLayoutHeader,
  NLayoutContent,
  NH3,
  NTag,
  NMenu,
  NIcon,
} from 'naive-ui'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { useTransferStore, normalizeTransfer } from './stores/transfer'
import { setupNotification, notifyError, notifySuccess, showChoice } from './composables/useNotification'
import DeviceList from './components/DeviceList.vue'
import TransferPanel from './components/TransferPanel.vue'
import ChatView from './views/ChatView.vue'
import HistoryView from './views/HistoryView.vue'
import SettingsView from './views/SettingsView.vue'

function renderIcon(icon: string) {
  return () => h(NIcon, null, { default: () => h('svg', { viewBox: '0 0 24 24', width: '18', height: '18', innerHTML: icon }) })
}

const menuOptions = [
  {
    label: 'Home',
    key: 'home',
    icon: renderIcon('<path fill="currentColor" d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z"/>'),
  },
  {
    label: 'Chat',
    key: 'chat',
    icon: renderIcon('<path fill="currentColor" d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H6l-2 2V4h16v12z"/>'),
  },
  {
    label: 'History',
    key: 'history',
    icon: renderIcon('<path fill="currentColor" d="M13 3c-4.97 0-9 4.03-9 9H1l3.89 3.89.07.14L9 12H6c0-3.87 3.13-7 7-7s7 3.13 7 7-3.13 7-7 7c-1.93 0-3.68-.79-4.94-2.06l-1.42 1.42C8.27 19.99 10.51 21 13 21c4.97 0 9-4.03 9-9s-4.03-9-9-9zm-1 5v5l4.28 2.54.72-1.21-3.5-2.08V8H12z"/>'),
  },
  {
    label: 'Settings',
    key: 'settings',
    icon: renderIcon('<path fill="currentColor" d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94L14.4 2.81c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.07.62-.07.94s.02.64.07.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z"/>'),
  },
]

const transferStore = useTransferStore()
setupNotification()

interface Peer {
  id: string
  name: string
  ip: string
  port: number
  online: boolean
  lastSeen: string
  manual?: boolean
}

const activeTab = ref('home')
const peers = ref<Peer[]>([])
const discovering = ref(false)

const onlineCount = computed(() => peers.value.filter((p) => p.online).length)

// Each EventsOn call returns its own unsubscribe function. EventsOff(name)
// removes every listener registered under that name, so using it in one
// component silently kills another's subscriptions.
let unsubscribeHandlers: Array<() => void> = []

onMounted(async () => {
  // Subscribe before the first fetch: RefreshPeers() blocks the Go side for
  // 500ms, and an inbound transfer arriving in that window would emit
  // transferStarted with nobody listening, so the task would never appear.
  setupEventListeners()
  await refreshPeers()
  await rehydrateTransfers()
})

onUnmounted(() => {
  teardownEventListeners()
})

function setupEventListeners() {
  teardownEventListeners()

  unsubscribeHandlers.push(
    EventsOn('transferEvent', (event: any) => {
      if (!event || !event.task) return
      const task = event.task

      switch (event.type) {
        case 'transferStarted':
          addOrUpdateTask(task)
          break

        case 'transferProgress':
          // status is included: the backend reports the authoritative value
          // here, and omitting it left resumed tasks stuck showing "failed".
          transferStore.updateTransfer(task.id, {
            bytesTransferred: task.bytesTransferred,
            speed: task.speed || 0,
            progress: task.progress || 0,
            status: task.status || 'transferring',
          })
          break

        case 'transferCompleted':
          transferStore.updateTransfer(task.id, {
            status: 'completed',
            progress: 100,
            endTime: task.endTime || new Date().toISOString(),
            speed: task.speed || 0,
          })
          break

        case 'transferFailed':
          transferStore.updateTransfer(task.id, {
            status: 'failed',
            endTime: task.endTime || new Date().toISOString(),
            error: task.error || 'Transfer failed',
          })
          notifyError(
            'Transfer Failed',
            `${task.fileName}: ${task.error || 'Unknown error'}`,
          )
          break

        case 'transferCancelled':
          transferStore.updateTransfer(task.id, {
            status: 'cancelled',
            endTime: task.endTime || new Date().toISOString(),
          })
          break

        case 'transferPaused':
          transferStore.updateTransfer(task.id, {
            status: 'paused',
          })
          break

        case 'transferResumed':
          transferStore.updateTransfer(task.id, {
            status: task.status || 'pending',
            error: undefined,
            endTime: '',
          })
          break
      }
    }),
  )

  unsubscribeHandlers.push(
    EventsOn('peerFound', (peer: Peer) => {
      const idx = peers.value.findIndex((p) => p.id === peer.id)
      if (idx >= 0) {
        peers.value[idx] = peer
      } else {
        peers.value.unshift(peer)
      }
    }),
  )

  unsubscribeHandlers.push(
    EventsOn('peerLost', (peerID: string) => {
      const peer = peers.value.find((p) => p.id === peerID)
      if (peer) {
        peer.online = false
      }
    }),
  )
}

function addOrUpdateTask(task: any) {
  const payload = normalizeTransfer(task)
  if (transferStore.getTransferById(payload.id)) {
    transferStore.updateTransfer(payload.id, payload)
  } else {
    transferStore.addTransfer(payload)
  }
}

function teardownEventListeners() {
  for (const off of unsubscribeHandlers) {
    try {
      off()
    } catch {
      // A listener may already be gone; nothing useful to do here.
    }
  }
  unsubscribeHandlers = []
}

// Pull tasks the backend already knows about. Without this, anything that
// happened before the first event listener was attached stays invisible until
// the next transfer starts.
async function rehydrateTransfers() {
  try {
    const { GetAllTasks } = await import('../wailsjs/go/main/App')
    const tasks = await GetAllTasks()
    for (const task of tasks ?? []) {
      addOrUpdateTask(task)
    }
  } catch (err) {
    notifyError('Failed to load transfers', String(err))
  }
}

function onPeerAdded(peer: Peer) {
  const idx = peers.value.findIndex((p) => p.id === peer.id)
  if (idx >= 0) {
    peers.value[idx] = peer
  } else {
    peers.value.unshift(peer)
  }
}

function onPeerRemoved(peerID: string) {
  peers.value = peers.value.filter((p) => p.id !== peerID)
}

async function refreshPeers() {
  try {
    discovering.value = true
    const { RefreshPeers } = await import('../wailsjs/go/main/App')
    const result = await RefreshPeers()
    // Merge rather than replace: RefreshPeers only returns what discovery saw,
    // which would drop manually added peers.
    const discovered = result ?? []
    const manual = peers.value.filter((p) => p.manual)
    const merged = new Map(manual.map((p) => [p.id, p]))
    for (const p of discovered) {
      merged.set(p.id, p)
    }
    peers.value = [...merged.values()]
  } catch (err) {
    notifyError('Failed to load peers', String(err))
  } finally {
    discovering.value = false
  }
}

// IPv6 literals must be bracketed before a port is appended.
function peerAddress(peer: Peer): string {
  const host = peer.ip.includes(':') ? `[${peer.ip}]` : peer.ip
  return `${host}:${peer.port}`
}

function baseName(path: string): string {
  const parts = path.split(/[\\/]/)
  return parts[parts.length - 1] || path
}

async function handleSendToPeer(peer: Peer) {
  try {
    const response = await showChoice(
      'Send to ' + peer.name,
      'Choose what to send:',
      'Send File',
      'Send Folder',
    )
    if (!response) return

    if (response === 'Send File') {
      const { SelectFile, SendFile } = await import('../wailsjs/go/main/App')
      const filePath = await SelectFile()
      if (!filePath) return

      await SendFile(peer.id, peer.name, peerAddress(peer), filePath)
      notifySuccess('File sent', baseName(filePath))
    } else if (response === 'Send Folder') {
      const { SelectDirectory, SendFolder } = await import('../wailsjs/go/main/App')
      const folderPath = await SelectDirectory()
      if (!folderPath) return

      await SendFolder(peer.id, peer.name, peerAddress(peer), folderPath)
      notifySuccess('Folder sent', baseName(folderPath))
    }
  } catch (err) {
    notifyError('Failed to send', String(err))
  }
}
</script>

<style>
.app-container {
  width: 100vw;
  height: 100vh;
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 24px;
  border-bottom: 1px solid #e5e5e5;
  height: 52px;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.header-nav {
  flex: 1;
  display: flex;
  justify-content: center;
  padding: 0 24px;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.layout-content {
  height: calc(100vh - 52px);
  overflow: auto;
}

.main-content {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  padding: 16px;
  height: calc(100vh - 52px);
}
</style>
