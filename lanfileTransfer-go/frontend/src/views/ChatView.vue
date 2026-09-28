<template>
  <div class="chat-view">
    <n-split direction="horizontal" :max="0.35" :min="0.2" :default-size="0.28">
      <template #1>
        <div class="chat-sidebar">
          <div class="chat-sidebar-header">
            <n-text strong>Contacts</n-text>
            <n-text depth="3" style="font-size: 12px">{{ onlinePeers.length }} online</n-text>
          </div>
          <n-scrollbar>
            <div
              v-for="peer in onlinePeers"
              :key="peer.id"
              class="chat-peer-item"
              :class="{ active: selectedPeer?.id === peer.id }"
              @click="selectPeer(peer)"
            >
              <n-avatar :size="36" round>
                {{ peer.name?.charAt(0)?.toUpperCase() || '?' }}
              </n-avatar>
              <div class="chat-peer-info">
                <n-text strong class="chat-peer-name">{{ peer.name }}</n-text>
                <n-text depth="3" style="font-size: 12px">{{ peer.ip }}</n-text>
              </div>
              <n-badge
                v-if="getUnreadCount(peer.id) > 0"
                :value="getUnreadCount(peer.id)"
                :max="99"
                dot
              />
            </div>
            <n-empty v-if="onlinePeers.length === 0" description="No online devices" size="small" class="chat-empty" />
          </n-scrollbar>
        </div>
      </template>
      <template #2>
        <div v-if="selectedPeer" class="chat-main">
          <div class="chat-header">
            <n-avatar :size="32" round>
              {{ selectedPeer.name?.charAt(0)?.toUpperCase() || '?' }}
            </n-avatar>
            <div class="chat-header-info">
              <n-text strong>{{ selectedPeer.name }}</n-text>
              <n-text depth="3" style="font-size: 12px">{{ selectedPeer.ip }}:{{ selectedPeer.port }}</n-text>
            </div>
          </div>
          <n-scrollbar ref="scrollbarRef" class="chat-messages" style="flex: 1">
            <div class="chat-messages-inner">
              <div
                v-for="msg in getMessages(selectedPeer.id)"
                :key="msg.id"
                class="chat-message"
                :class="msg.isSelf ? 'chat-message-self' : 'chat-message-peer'"
              >
                <div class="chat-message-bubble">
                  <n-text>{{ msg.content }}</n-text>
                </div>
                <n-text depth="3" class="chat-message-time">{{ formatTime(msg.timestamp) }}</n-text>
              </div>
            </div>
          </n-scrollbar>
          <div class="chat-input-area">
            <n-input
              v-model:value="inputText"
              type="textarea"
              placeholder="Type a message..."
              :autosize="{ minRows: 1, maxRows: 4 }"
              @keydown="handleKeydown"
            />
            <n-button type="primary" :disabled="!inputText.trim()" @click="sendMessage">
              <template #icon>
                <n-icon><svg viewBox="0 0 24 24" width="18" height="18"><path fill="currentColor" d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/></svg></n-icon>
              </template>
              Send
            </n-button>
          </div>
        </div>
        <div v-else class="chat-placeholder">
          <n-empty description="Select a contact to start chatting" size="large" />
        </div>
      </template>
    </n-split>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onUnmounted } from 'vue'
import {
  NSplit,
  NText,
  NScrollbar,
  NAvatar,
  NBadge,
  NEmpty,
  NInput,
  NButton,
  NIcon,
} from 'naive-ui'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { msgError } from '../composables/useNotification'
import * as wails from '../../wailsjs/go/main/App'

interface Peer {
  id: string
  name: string
  ip: string
  port: number
  online: boolean
  lastSeen: string
}

interface ChatMessage {
  id: string
  peerId: string
  peerName: string
  content: string
  timestamp: number
  isSelf: boolean
}

const peers = ref<Peer[]>([])
const selectedPeer = ref<Peer | null>(null)
const inputText = ref('')
const messages = ref<Map<string, ChatMessage[]>>(new Map())
const unreadCounts = ref<Map<string, number>>(new Map())
const msgIdCounter = ref(0)

const onlinePeers = computed(() => {
  return peers.value.filter(p => p.online)
})

function selectPeer(peer: any) {
  selectedPeer.value = peer
  unreadCounts.value.set(peer.id, 0)
  if (!messages.value.has(peer.id)) {
    messages.value.set(peer.id, [])
  }
}

function getMessages(peerId: string): ChatMessage[] {
  return messages.value.get(peerId) || []
}

function getUnreadCount(peerId: string): number {
  return unreadCounts.value.get(peerId) || 0
}

function formatTime(ts: number): string {
  const d = new Date(ts * 1000)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    sendMessage()
  }
}

async function sendMessage() {
  if (!selectedPeer.value || !inputText.value.trim()) return

  const content = inputText.value.trim()
  const peerId = selectedPeer.value.id

  const msg: ChatMessage = {
    id: String(++msgIdCounter.value),
    peerId,
    peerName: 'Me',
    content,
    timestamp: Math.floor(Date.now() / 1000),
    isSelf: true,
  }

  if (!messages.value.has(peerId)) {
    messages.value.set(peerId, [])
  }
  messages.value.get(peerId)!.push(msg)

  try {
    await (wails as any).SendMessage(peerId, content)
    inputText.value = ''
    scrollToBottom()
  } catch (err) {
    msgError('Failed to send message')
  }
}

function handleIncomingMessage(data: any) {
  const senderId = data.senderId
  const senderName = data.senderName
  const content = data.message
  const timestamp = data.timestamp

  if (!messages.value.has(senderId)) {
    messages.value.set(senderId, [])
  }

  const msg: ChatMessage = {
    id: String(++msgIdCounter.value),
    peerId: senderId,
    peerName: senderName,
    content,
    timestamp,
    isSelf: false,
  }
  messages.value.get(senderId)!.push(msg)

  if (selectedPeer.value?.id !== senderId) {
    const current = unreadCounts.value.get(senderId) || 0
    unreadCounts.value.set(senderId, current + 1)
  } else {
    scrollToBottom()
  }
}

function scrollToBottom() {
  nextTick(() => {
    const scrollbar = document.querySelector('.chat-messages .n-scrollbar-container')
    if (scrollbar) {
      scrollbar.scrollTop = scrollbar.scrollHeight
    }
  })
}

async function loadPeers() {
  try {
    const result = await wails.RefreshPeers()
    peers.value = result
  } catch (err) {
    console.error('Failed to load peers:', err)
  }
}

function handlePeerFound(peer: Peer) {
  const idx = peers.value.findIndex(p => p.id === peer.id)
  if (idx >= 0) {
    peers.value[idx] = peer
  } else {
    peers.value.unshift(peer)
  }
}

function handlePeerLost(peerID: string) {
  const peer = peers.value.find(p => p.id === peerID)
  if (peer) {
    peer.online = false
  }
}

onMounted(() => {
  loadPeers()
  EventsOn('chatMessage', handleIncomingMessage)
  EventsOn('peerFound', handlePeerFound)
  EventsOn('peerLost', handlePeerLost)
})

onUnmounted(() => {
  EventsOff('chatMessage')
  EventsOff('peerFound')
  EventsOff('peerLost')
})
</script>

<style scoped>
.chat-view {
  height: 100%;
  display: flex;
}

.chat-sidebar {
  height: 100%;
  border-right: 1px solid #e8e8e8;
  display: flex;
  flex-direction: column;
}

.chat-sidebar-header {
  padding: 12px 16px;
  border-bottom: 1px solid #e8e8e8;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.chat-peer-item {
  display: flex;
  align-items: center;
  padding: 10px 16px;
  cursor: pointer;
  transition: background 0.2s;
  gap: 10px;
}

.chat-peer-item:hover {
  background: #f5f5f5;
}

.chat-peer-item.active {
  background: #e8f4ff;
}

.chat-peer-info {
  flex: 1;
  min-width: 0;
}

.chat-peer-name {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-empty {
  padding: 20px 0;
}

.chat-main {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.chat-header {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid #e8e8e8;
  gap: 10px;
}

.chat-header-info {
  display: flex;
  flex-direction: column;
}

.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}

.chat-messages-inner {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chat-message {
  display: flex;
  flex-direction: column;
  max-width: 70%;
}

.chat-message-self {
  align-self: flex-end;
  align-items: flex-end;
}

.chat-message-peer {
  align-self: flex-start;
  align-items: flex-start;
}

.chat-message-bubble {
  padding: 8px 12px;
  border-radius: 12px;
  word-break: break-word;
}

.chat-message-self .chat-message-bubble {
  background: #18a058;
  color: white;
  border-bottom-right-radius: 4px;
}

.chat-message-peer .chat-message-bubble {
  background: #f0f0f0;
  border-bottom-left-radius: 4px;
}

.chat-message-time {
  font-size: 11px;
  margin-top: 2px;
}

.chat-input-area {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid #e8e8e8;
  align-items: flex-end;
}

.chat-input-area .n-input {
  flex: 1;
}

.chat-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}
</style>