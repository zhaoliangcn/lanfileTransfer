<template>
  <div class="home-view">
    <n-space vertical :size="16">
      <n-card title="Quick Actions" size="small">
        <n-space>
          <n-button type="primary" @click="handleSendFile">
            <template #icon>
              <n-icon><svg viewBox="0 0 24 24" width="18" height="18"><path fill="currentColor" d="M9 16h6v-6h4l-7-7-7 7h4zm-4 2h14v2H5z"/></svg></n-icon>
            </template>
            Send File
          </n-button>
          <n-button @click="handleSendFolder">
            <template #icon>
              <n-icon><svg viewBox="0 0 24 24" width="18" height="18"><path fill="currentColor" d="M20 6h-8l-2-2H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2zm0 12H4V8h16v10z"/></svg></n-icon>
            </template>
            Send Folder
          </n-button>
          <n-button @click="handleReceiveFile">
            <template #icon>
              <n-icon><svg viewBox="0 0 24 24" width="18" height="18"><path fill="currentColor" d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z"/></svg></n-icon>
            </template>
            Receive File
          </n-button>
        </n-space>
      </n-card>

      <n-card title="Statistics" size="small">
        <n-descriptions :column="4" size="small">
          <n-descriptions-item label="Total Transfers">
            <n-number-animation :from="0" :to="store.completedCount + store.failedCount" />
          </n-descriptions-item>
          <n-descriptions-item label="Completed">
            <n-number-animation :from="0" :to="store.completedCount" />
          </n-descriptions-item>
          <n-descriptions-item label="Failed">
            <n-number-animation :from="0" :to="store.failedCount" />
          </n-descriptions-item>
          <n-descriptions-item label="Active">
            <n-number-animation :from="0" :to="store.activeCount" />
          </n-descriptions-item>
        </n-descriptions>
      </n-card>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import {
  NSpace, NCard, NButton, NIcon, NDescriptions,
  NDescriptionsItem, NNumberAnimation,
} from 'naive-ui'
import { useTransferStore } from '../stores/transfer'

const store = useTransferStore()

async function handleSendFile() {
  try {
    const { SelectFile } = await import('../../wailsjs/go/main/App')
    const filePath = await SelectFile()
    if (filePath) {
      console.log('Selected file:', filePath)
    }
  } catch (err) {
    console.error('Failed to select file:', err)
  }
}

async function handleSendFolder() {
  try {
    const { SelectDirectory } = await import('../../wailsjs/go/main/App')
    const dirPath = await SelectDirectory()
    if (dirPath) {
      console.log('Selected folder:', dirPath)
    }
  } catch (err) {
    console.error('Failed to select folder:', err)
  }
}

async function handleReceiveFile() {
  try {
    const { SelectDirectory } = await import('../../wailsjs/go/main/App')
    const dirPath = await SelectDirectory()
    if (dirPath) {
      console.log('Selected directory:', dirPath)
    }
  } catch (err) {
    console.error('Failed to select directory:', err)
  }
}
</script>