<template>
  <div class="settings-view">
    <n-card title="Settings" :bordered="true">
      <n-form ref="formRef" :model="formData" :rules="rules" label-placement="left" label-width="200">
        <n-divider title-position="left">Transfer Settings</n-divider>

        <n-form-item label="Default Save Path" path="defaultSavePath">
          <n-input-group>
            <n-input v-model:value="formData.defaultSavePath" placeholder="Select save directory" readonly />
            <n-button @click="selectSavePath">Browse</n-button>
          </n-input-group>
        </n-form-item>

        <n-form-item label="Auto Receive Files">
          <n-switch v-model:value="formData.autoReceive" />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              When off, incoming transfers are refused instead of being written to disk.
            </n-text>
          </template>
        </n-form-item>

        <n-form-item label="Max Concurrent Transfers" path="maxConcurrentTransfers">
          <n-input-number
            v-model:value="formData.maxConcurrentTransfers"
            :min="1"
            :max="10"
            :step="1"
          />
        </n-form-item>

        <n-form-item label="Chunk Size" path="chunkSize">
          <n-select
            v-model:value="formData.chunkSize"
            :options="chunkSizeOptions"
          />
        </n-form-item>

        <n-divider title-position="left">Network Settings</n-divider>

        <n-alert type="info" :show-icon="true" style="margin-bottom: 16px">
          Discovery uses UDP broadcast, which routers do not forward. A device on another
          subnet must be added by address from the Devices panel, and the listen port
          (TCP) plus the discovery port (UDP) must be reachable on its firewall.
        </n-alert>

        <n-form-item label="Listen Port" path="listenPort">
          <n-input-number
            v-model:value="formData.listenPort"
            :min="1024"
            :max="65535"
            :step="1"
          />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              Also used as the discovery port. Takes effect after a restart.
            </n-text>
          </template>
        </n-form-item>

        <n-form-item label="Discovery Interval" path="discoveryInterval">
          <n-input-number
            v-model:value="formData.discoveryInterval"
            :min="1"
            :max="60"
            :step="1"
          >
            <template #suffix>seconds</template>
          </n-input-number>
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              How often presence is broadcast. Takes effect after a restart.
            </n-text>
          </template>
        </n-form-item>

        <n-divider title-position="left">About</n-divider>

        <n-descriptions :column="1" size="small" label-placement="left">
          <n-descriptions-item label="Application">LanFileTransfer</n-descriptions-item>
          <n-descriptions-item label="Version">1.0.0</n-descriptions-item>
          <n-descriptions-item label="Technology">Go + Wails + Vue 3</n-descriptions-item>
        </n-descriptions>

        <div class="form-actions">
          <n-space>
            <n-button type="primary" @click="saveConfig" :loading="saving">
              Save
            </n-button>
            <n-button @click="resetConfig">
              Reset to Defaults
            </n-button>
          </n-space>
        </div>
      </n-form>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import {
  NCard, NForm, NFormItem, NInput, NInputGroup, NInputNumber,
  NButton, NSwitch, NSelect, NSpace, NDivider, NAlert, NText,
  NDescriptions, NDescriptionsItem, useMessage,
  type FormInst,
} from 'naive-ui'
import { GetConfig, SaveConfig, SelectDirectory } from '../../wailsjs/go/main/App'

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)

const chunkSizeOptions = [
  { label: '16 KB', value: 16 * 1024 },
  { label: '32 KB', value: 32 * 1024 },
  { label: '64 KB (Default)', value: 64 * 1024 },
  { label: '128 KB', value: 128 * 1024 },
  { label: '256 KB', value: 256 * 1024 },
  { label: '512 KB', value: 512 * 1024 },
  { label: '1 MB', value: 1024 * 1024 },
]

interface SettingsForm {
  defaultSavePath: string
  autoReceive: boolean
  maxConcurrentTransfers: number
  chunkSize: number
  listenPort: number
  discoveryInterval: number
}

const defaultForm: SettingsForm = {
  defaultSavePath: '',
  autoReceive: true,
  maxConcurrentTransfers: 3,
  chunkSize: 64 * 1024,
  listenPort: 9876,
  discoveryInterval: 5,
}

const formData = reactive<SettingsForm>({ ...defaultForm })

const rules = {
  defaultSavePath: [
    { required: true, message: 'Please select a save path', trigger: 'blur' },
  ],
  maxConcurrentTransfers: [
    { required: true, message: 'Required', trigger: 'blur' },
    { min: 1, max: 10, message: 'Must be 1-10', trigger: 'blur' },
  ],
  chunkSize: [
    { required: true, message: 'Required', trigger: 'blur' },
  ],
  listenPort: [
    { required: true, message: 'Required', trigger: 'blur' },
    { min: 1024, max: 65535, message: 'Must be 1024-65535', trigger: 'blur' },
  ],
  discoveryInterval: [
    { required: true, message: 'Required', trigger: 'blur' },
    { min: 1, max: 60, message: 'Must be 1-60', trigger: 'blur' },
  ],
}

onMounted(async () => {
  try {
    const cfg = await GetConfig()
    if (cfg) {
      formData.defaultSavePath = cfg.defaultSavePath || ''
      formData.autoReceive = cfg.autoReceive ?? true
      formData.maxConcurrentTransfers = cfg.maxConcurrentTransfers || 3
      formData.chunkSize = cfg.chunkSize || 64 * 1024
      formData.listenPort = cfg.listenPort || 9876
      formData.discoveryInterval = cfg.discoveryInterval || 5
    }
  } catch (err) {
    console.error('Failed to load config:', err)
    // Warn explicitly: a silent fallback here used to make the user believe the
    // saved settings were active.
    message.warning('Could not read the saved settings; showing defaults. Saving now would overwrite them.')
  }
})

async function selectSavePath() {
  try {
    const dirPath = await SelectDirectory()
    if (dirPath) {
      formData.defaultSavePath = dirPath
    }
  } catch (err) {
    message.error(`Could not open the directory picker: ${String(err)}`)
  }
}

async function saveConfig() {
  // The rules were defined but never executed, so an empty save path could be
  // written. Run them before touching the backend.
  try {
    await formRef.value?.validate()
  } catch {
    message.error('Please fix the highlighted fields')
    return
  }

  saving.value = true
  try {
    const currentCfg = await GetConfig()
    if (!currentCfg) {
      message.error('Could not read the current settings; not saving to avoid overwriting them')
      return
    }

    await SaveConfig({
      ...currentCfg,
      defaultSavePath: formData.defaultSavePath,
      autoReceive: formData.autoReceive,
      maxConcurrentTransfers: formData.maxConcurrentTransfers,
      chunkSize: formData.chunkSize,
      listenPort: formData.listenPort,
      discoveryInterval: formData.discoveryInterval,
    })

    // Be honest about what still needs a restart.
    const { PendingRestartFields } = await import('../../wailsjs/go/main/App')
    const pending = (await PendingRestartFields()) ?? []
    if (pending.length > 0) {
      const labels = pending
        .map((f) => (f === 'listenPort' ? 'Listen port' : 'Discovery interval'))
        .join(' and ')
      message.warning(
        `Settings saved. ${labels} ${pending.length > 1 ? 'take' : 'takes'} effect after a restart.`,
        { duration: 8000 },
      )
    } else {
      message.success('Settings saved')
    }
  } catch (err) {
    message.error(`Failed to save settings: ${String(err)}`)
  } finally {
    saving.value = false
  }
}

function resetConfig() {
  Object.assign(formData, { ...defaultForm })
  message.info('Settings reset to defaults (not saved)')
}
</script>

<style scoped>
.settings-view {
  padding: 16px;
  max-width: 700px;
}

.form-actions {
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid #e5e5e5;
}
</style>