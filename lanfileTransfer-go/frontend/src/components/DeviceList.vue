<template>
  <n-card title="Devices" :bordered="true" class="device-list-card">
    <template #header-extra>
      <n-space size="small">
        <n-button quaternary circle size="small" :loading="loading" @click="$emit('refresh')">
          <template #icon>
            <n-icon><svg viewBox="0 0 24 24" width="16" height="16"><path fill="currentColor" d="M12 4V1L8 5l4 4V6c3.31 0 6 2.69 6 6 0 1.01-.25 1.97-.7 2.8l1.46 1.46C19.54 15.03 20 13.57 20 12c0-4.42-3.58-8-8-8zm0 14c-3.31 0-6-2.69-6-6 0-1.01.25-1.97.7-2.8L5.24 7.74C4.46 8.97 4 10.43 4 12c0 4.42 3.58 8 8 8v3l4-4-4-4v3z"/></svg></n-icon>
          </template>
        </n-button>
        <n-button quaternary circle size="small" @click="openAddPeer">
          <template #icon>
            <n-icon><svg viewBox="0 0 24 24" width="16" height="16"><path fill="currentColor" d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/></svg></n-icon>
          </template>
        </n-button>
      </n-space>
    </template>

    <n-spin :show="loading">
      <n-empty v-if="filteredPeers.length === 0" description="No devices found">
        <template #extra>
          <n-space vertical size="small" style="align-items: center">
            <n-button size="small" @click="$emit('refresh')">Refresh</n-button>
            <n-text depth="3" style="font-size: 12px; max-width: 260px; display: block; text-align: center">
              Discovery uses UDP broadcast, which routers do not forward. Devices on another
              subnet can be added by address.
            </n-text>
            <n-button size="small" secondary @click="openAddPeer">Add device by address</n-button>
          </n-space>
        </template>
      </n-empty>

      <n-list v-else>
        <n-list-item
          v-for="peer in filteredPeers"
          :key="peer.id"
          @contextmenu.prevent="handleContextMenu($event, peer)"
        >
          <template #prefix>
            <n-badge :type="peer.online ? 'success' : 'default'" dot />
          </template>
          <n-thing :title="peer.name" :description="`${peer.ip}:${peer.port}`">
            <template #footer>
              <n-space size="small">
                <n-tag v-if="peer.online" size="tiny" type="success">Online</n-tag>
                <n-tag v-else size="tiny" type="default">Offline</n-tag>
                <n-tag v-if="peer.manual" size="tiny" type="info">Manual</n-tag>
              </n-space>
            </template>
            <template #action>
              <n-space size="small">
                <n-button
                  size="small"
                  type="primary"
                  :disabled="!peer.online"
                  @click="$emit('send', peer)"
                >
                  Send
                </n-button>
                <n-button
                  size="small"
                  quaternary
                  @click="showDetail(peer)"
                >
                  Details
                </n-button>
              </n-space>
            </template>
          </n-thing>
        </n-list-item>
      </n-list>
    </n-spin>

    <n-dropdown
      placement="bottom-start"
      trigger="manual"
      :show="contextMenuVisible"
      :x="contextMenuX"
      :y="contextMenuY"
      :options="contextMenuOptions"
      @clickoutside="contextMenuVisible = false"
      @select="handleContextMenuSelect"
    />

    <n-modal v-model:show="detailModalVisible" preset="card" title="Device Details" style="width: 400px">
      <n-descriptions v-if="selectedPeer" :column="1" size="small" label-placement="left" bordered>
        <n-descriptions-item label="Name">{{ selectedPeer.name }}</n-descriptions-item>
        <n-descriptions-item label="IP Address">{{ selectedPeer.ip }}</n-descriptions-item>
        <n-descriptions-item label="Port">{{ selectedPeer.port }}</n-descriptions-item>
        <n-descriptions-item label="Status">
          <n-tag :type="selectedPeer.online ? 'success' : 'default'" size="small">
            {{ selectedPeer.online ? 'Online' : 'Offline' }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="Source">
          {{ selectedPeer.manual ? 'Added manually' : 'Discovered on this network' }}
        </n-descriptions-item>
        <n-descriptions-item label="Device ID">
          <n-ellipsis style="max-width: 280px">{{ selectedPeer.id }}</n-ellipsis>
        </n-descriptions-item>
        <n-descriptions-item label="Last Seen">{{ selectedPeer.lastSeen || 'N/A' }}</n-descriptions-item>
      </n-descriptions>
      <template #footer>
        <n-space justify="end">
          <n-button
            v-if="selectedPeer && selectedPeer.manual"
            type="error"
            secondary
            @click="removeSelected"
          >
            Remove
          </n-button>
          <n-button
            v-if="selectedPeer && selectedPeer.online"
            type="primary"
            @click="sendToSelected"
          >
            Send File
          </n-button>
          <n-button @click="detailModalVisible = false">Close</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal
      v-model:show="addPeerVisible"
      preset="card"
      title="Add device by address"
      style="width: 420px"
    >
      <n-form label-placement="top">
        <n-form-item label="IP address or hostname">
          <n-input
            v-model:value="addPeerForm.address"
            placeholder="192.168.0.100"
            :status="addPeerError ? 'error' : undefined"
            @keyup.enter="submitAddPeer"
          />
        </n-form-item>
        <n-form-item label="Port">
          <n-input-number
            v-model:value="addPeerForm.port"
            :min="1"
            :max="65535"
            placeholder="9876"
            style="width: 100%"
          />
        </n-form-item>
        <n-form-item label="Display name (optional)">
          <n-input v-model:value="addPeerForm.name" placeholder="Office PC" @keyup.enter="submitAddPeer" />
        </n-form-item>
      </n-form>

      <n-alert v-if="addPeerError" type="error" :show-icon="true" style="margin-top: 4px">
        {{ addPeerError }}
      </n-alert>
      <n-alert v-else type="info" :show-icon="true" style="margin-top: 4px">
        Broadcast discovery cannot cross a router. The device must still accept TCP
        connections on the port above (allow it through its firewall).
      </n-alert>

      <template #footer>
        <n-space justify="end">
          <n-button @click="addPeerVisible = false">Cancel</n-button>
          <n-button
            type="primary"
            :loading="probing"
            :disabled="!addPeerForm.address"
            @click="submitAddPeer"
          >
            Add device
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </n-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  NCard, NSpin, NEmpty, NButton, NList, NListItem,
  NBadge, NThing, NTag, NIcon, NSpace, NModal,
  NDescriptions, NDescriptionsItem, NEllipsis, NDropdown,
  NForm, NFormItem, NInput, NInputNumber, NAlert, NText,
  useDialog, useMessage,
} from 'naive-ui'

interface Peer {
  id: string
  name: string
  ip: string
  port: number
  online: boolean
  lastSeen: string
  manual?: boolean
}

const props = defineProps<{
  peers: Peer[]
  loading: boolean
}>()

const emit = defineEmits<{
  send: [peer: Peer]
  refresh: []
  'peer-added': [peer: Peer]
  'peer-removed': [peerID: string]
}>()

const message = useMessage()
const dialog = useDialog()

const filteredPeers = computed(() =>
  [...props.peers].sort((a, b) => {
    if (a.online && !b.online) return -1
    if (!a.online && b.online) return 1
    return (a.name ?? '').localeCompare(b.name ?? '')
  })
)

const selectedPeer = ref<Peer | null>(null)
const detailModalVisible = ref(false)

const addPeerVisible = ref(false)
const probing = ref(false)
const addPeerError = ref('')
const addPeerForm = ref<{ address: string; port: number | null; name: string }>({
  address: '',
  port: null,
  name: '',
})

const contextMenuVisible = ref(false)
const contextMenuX = ref(0)
const contextMenuY = ref(0)
const contextMenuPeer = ref<Peer | null>(null)

const contextMenuOptions = computed(() => [
  {
    label: 'Send File',
    key: 'send',
    disabled: !contextMenuPeer.value?.online,
  },
  {
    label: 'View Details',
    key: 'details',
  },
  { type: 'divider' },
  {
    label: 'Remote Shutdown',
    key: 'shutdown',
    disabled: !contextMenuPeer.value?.online,
  },
  {
    label: 'Remote Restart',
    key: 'restart',
    disabled: !contextMenuPeer.value?.online,
  },
  {
    label: 'Remove Device',
    key: 'remove',
    disabled: !contextMenuPeer.value?.manual,
  },
])

function handleContextMenu(e: MouseEvent, peer: Peer) {
  contextMenuPeer.value = peer
  contextMenuX.value = e.clientX
  contextMenuY.value = e.clientY
  contextMenuVisible.value = true
}

function handleContextMenuSelect(key: string) {
  contextMenuVisible.value = false
  const peer = contextMenuPeer.value
  if (!peer) return

  if (key === 'send') {
    emit('send', peer)
  } else if (key === 'details') {
    showDetail(peer)
  } else if (key === 'shutdown') {
    confirmAndSendCommand('shutdown', peer)
  } else if (key === 'restart') {
    confirmAndSendCommand('restart', peer)
  } else if (key === 'remove') {
    confirmRemove(peer)
  }
}

// Shutting down or restarting someone else's machine is destructive and
// irreversible. This dialog was previously named "confirm..." but never
// actually confirmed anything, so a stray click powered a colleague's PC off.
function confirmAndSendCommand(command: 'shutdown' | 'restart', peer: Peer) {
  const label = command === 'shutdown' ? 'shut down' : 'restart'

  dialog.warning({
    title: `Remote ${command}`,
    content: `Send a request to ${label} "${peer.name}" (${peer.ip})?`,
    positiveText: `Yes, ${label} it`,
    negativeText: 'Cancel',
    onPositiveClick: async () => {
      try {
        const { RemoteShutdown, RemoteRestart } = await import('../../wailsjs/go/main/App')
        if (command === 'shutdown') {
          await RemoteShutdown(peer.id)
        } else {
          await RemoteRestart(peer.id)
        }
        message.success(`Remote ${command} request sent to ${peer.name}`)
      } catch (err) {
        message.error(`Failed to send ${command} request: ${String(err)}`)
      }
    },
  })
}

function confirmRemove(peer: Peer) {
  dialog.warning({
    title: 'Remove device',
    content: `Remove "${peer.name}" (${peer.ip}) from the device list?`,
    positiveText: 'Remove',
    negativeText: 'Cancel',
    onPositiveClick: () => removePeer(peer),
  })
}

async function removeSelected() {
  const peer = selectedPeer.value
  if (!peer) return
  detailModalVisible.value = false
  await removePeer(peer)
}

async function removePeer(peer: Peer) {
  try {
    const { RemoveManualPeer } = await import('../../wailsjs/go/main/App')
    const removed = await RemoveManualPeer(peer.id)
    if (removed) {
      message.success(`Removed ${peer.name}`)
      emit('peer-removed', peer.id)
    } else {
      message.warning(`${peer.name} was not added manually and cannot be removed here`)
    }
  } catch (err) {
    message.error(`Failed to remove device: ${String(err)}`)
  }
}

function openAddPeer() {
  addPeerError.value = ''
  addPeerForm.value = { address: '', port: null, name: '' }
  addPeerVisible.value = true
}

async function submitAddPeer() {
  const address = addPeerForm.value.address.trim()
  if (!address) return

  addPeerError.value = ''
  probing.value = true
  const port = addPeerForm.value.port ?? 0

  try {
    const { AddManualPeer, ProbePeer } = await import('../../wailsjs/go/main/App')

    // Fail early with a clear message instead of letting the first transfer
    // time out minutes later.
    try {
      await ProbePeer(address, port)
    } catch (err) {
      addPeerError.value = `${String(err)}. The device must be running and its firewall must allow the port.`
      return
    }

    const peer = await AddManualPeer(addPeerForm.value.name.trim(), address, port)
    addPeerVisible.value = false
    message.success(`Added ${peer.name}`)
    emit('peer-added', peer)
  } catch (err) {
    addPeerError.value = String(err)
  } finally {
    probing.value = false
  }
}

function showDetail(peer: Peer) {
  selectedPeer.value = peer
  detailModalVisible.value = true
}

function sendToSelected() {
  detailModalVisible.value = false
  if (selectedPeer.value) {
    emit('send', selectedPeer.value)
  }
}
</script>

<style scoped>
.device-list-card {
  height: 100%;
  overflow: auto;
}
</style>
