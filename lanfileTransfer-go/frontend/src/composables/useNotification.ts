// src/composables/useNotification.ts
import { createDiscreteApi } from 'naive-ui'

// 创建完全独立于 Vue 组件树的 API 实例
// 这样可以在任何地方（组件内外）安全调用，无需 Provider 上下文
const { message, notification, dialog } = createDiscreteApi([
  'message',
  'notification',
  'dialog',
])

// ---------- Notification 风格 API ----------
export function notifyError(title: string, content?: string, duration = 4000) {
  notification.error({
    title,
    content: content || '',
    duration,
  })
}

export function notifySuccess(title: string, content?: string, duration = 3000) {
  notification.success({
    title,
    content: content || '',
    duration,
  })
}

export function notifyWarning(title: string, content?: string, duration = 3500) {
  notification.warning({
    title,
    content: content || '',
    duration,
  })
}

export function notifyInfo(title: string, content?: string, duration = 3000) {
  notification.info({
    title,
    content: content || '',
    duration,
  })
}

// ---------- Message 风格 API ----------
export function msgError(content: string, duration = 3000) {
  message.error(content, { duration })
}

export function msgSuccess(content: string, duration = 2000) {
  message.success(content, { duration })
}

export function msgWarning(content: string, duration = 3000) {
  message.warning(content, { duration })
}

export function msgInfo(content: string, duration = 2000) {
  message.info(content, { duration })
}

// ---------- Dialog 风格 API ----------
export function showConfirm(title: string, content: string): Promise<boolean> {
  return new Promise((resolve) => {
    dialog.warning({
      title,
      content,
      positiveText: 'Confirm',
      negativeText: 'Cancel',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
    })
  })
}

export function showChoice(
  title: string,
  content: string,
  positiveText: string,
  negativeText: string,
): Promise<string | null> {
  return new Promise((resolve) => {
    dialog.info({
      title,
      content,
      positiveText,
      negativeText,
      onPositiveClick: () => resolve(positiveText),
      onNegativeClick: () => resolve(negativeText),
      onClose: () => resolve(null),
    })
  })
}

// 为了兼容旧代码，保留一个空的 setupNotification 函数
// 如果原有代码调用了它，不会报错
export function setupNotification() {
  // 无需任何操作，因为 API 已经通过 createDiscreteApi 创建好
  console.debug('Notification APIs ready (via createDiscreteApi)')
}
