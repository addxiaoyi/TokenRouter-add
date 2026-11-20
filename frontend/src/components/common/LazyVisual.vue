<script setup lang="ts">
import { onBeforeUnmount, onErrorCaptured, onMounted, ref } from 'vue'
import { observeVisualActivity } from '@/utils/visualActivity'

// LazyVisual.vue delays decorative component creation until its reserved area
// approaches the viewport. The default slot is not evaluated before activation,
// so Vue async components inside it defer their module downloads and renderer
// initialization until the browser has an idle opportunity. Callers reserve the
// area's dimensions and supply its static fallback; neither loading nor a failed
// visual changes surrounding layout.
//
// Activation is permanent for this mounted boundary. Its child renderer owns
// viewport pausing after creation, retaining simulation state when visitors scroll
// away and return. A failed visual is removed while its fallback remains visible;
// errors are logged with component context instead of breaking the page's content.

// Admission limits preload only nearby artwork and keep busy documents from
// deferring their decorative initialization indefinitely after the foreground has painted.
const VISUAL_PRELOAD_MARGIN = '240px'
const VISUAL_IDLE_TIMEOUT_MS = 1000

// target and activated own viewport admission, while loaded retires the fallback
// only after async children mount. failed restores that fallback after an error.
// stopWatching and scheduling handles release observers and pending idle admission.
const target = ref<HTMLElement | null>(null)
const activated = ref(false)
const failed = ref(false)
const loaded = ref(false)
let stopWatching: (() => void) | undefined
let idleHandle: number | undefined
let timerHandle: ReturnType<typeof setTimeout> | undefined

// activate releases the decorative slot after viewport admission and browser idle.
// The retained flag avoids importing its async component again after later scrolling.
function activate() {
  idleHandle = undefined
  timerHandle = undefined
  if (document.hidden) return
  activated.value = true
  stopWatching?.()
}

// visualLoaded removes static artwork after all decorative async children mount.
function visualLoaded() {
  loaded.value = true
}

// cancelAdmission cancels a pending idle or compatibility-timer activation.
// Keeping the boundary unmounted lets a later visible notification schedule work
// again without downloading a visual for an inactive page.
function cancelAdmission() {
  if (idleHandle !== undefined) window.cancelIdleCallback(idleHandle)
  if (timerHandle !== undefined) clearTimeout(timerHandle)
  idleHandle = undefined
  timerHandle = undefined
}

onMounted(() => {
  if (!target.value) return
  // Preload close to the viewport rather than at page mount, preserving time for
  // the foreground interface before expensive shader or physics modules are requested.
  stopWatching = observeVisualActivity(
    target.value,
    (active) => {
      if (!active) {
        cancelAdmission()
        return
      }
      if (activated.value || idleHandle !== undefined || timerHandle !== undefined) return
      // An idle opportunity keeps shader setup behind foreground interactions.
      // The timeout bounds deferred decoration during continuously busy user activity.
      if (typeof window.requestIdleCallback === 'function') {
        idleHandle = window.requestIdleCallback(activate, { timeout: VISUAL_IDLE_TIMEOUT_MS })
      } else {
        // Browsers without idle callbacks defer to the next task so mounting
        // still yields to the current interface work before creating decorative renderers.
        timerHandle = setTimeout(activate, 0)
      }
    },
    VISUAL_PRELOAD_MARGIN,
  )
})

onErrorCaptured((error, _instance, info) => {
  // A decorative import or renderer failure must preserve page navigation and
  // readable content. Keep the supplied static artwork and record the actual cause.
  failed.value = true
  console.error(`[LazyVisual.vue] Decorative visual failed during ${info}`, error)
  return false
})

onBeforeUnmount(() => {
  // Admission can precede idle execution; cancel both scheduling variants before
  // unmount so a queued callback cannot create decorative work for a retired page.
  stopWatching?.()
  cancelAdmission()
})
</script>

<template>
  <div ref="target" class="h-full w-full">
    <div v-if="!loaded || failed" class="absolute inset-0" aria-hidden="true">
      <slot name="fallback" />
    </div>
    <Suspense v-if="activated && !failed" @resolve="visualLoaded">
      <div class="absolute inset-0">
        <slot />
      </div>
      <template #fallback><span /></template>
    </Suspense>
  </div>
</template>
