/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}

// Wails v2 injects its API onto `window.runtime`, not onto `window` directly.
// The previous declarations here described a non-existent `window.EventsOn` /
// `window.EventsOff` pair with the wrong signatures, which is why the dead
// composables in src/composables silently no-op. Import the real bindings from
// '../wailsjs/runtime/runtime' instead; the declarations below only cover the
// globals the runtime actually creates.
//
// Wails regenerates frontend/wailsjs/ on every `wails dev` / `wails build`, so
// these shapes are derived from wails v2's runtime contract.

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type WailsCallback = (...args: any[]) => void

interface Window {
  runtime?: {
    EventsOn: (eventName: string, callback: WailsCallback) => () => void
    EventsOnMultiple: (eventName: string, callbacks: WailsCallback[]) => () => void
    EventsEmit: (eventName: string, ...data: unknown[]) => void
    EventsOff: (eventName: string, ...additionalEventNames: string[]) => void
    WindowReload: () => void
    WindowReloadApp: () => void
    Quit: () => void
  }
  go?: Record<string, Record<string, Record<string, (...args: never[]) => unknown>>>
}
