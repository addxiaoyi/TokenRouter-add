// browser-auto-update.js reloads idle browser tabs after the deployed frontend
// changes. The reverse proxy injects this independent script into SPA documents,
// so checking an already deployed build does not replace application code or
// require a backend restart. Only same-origin Vite module and stylesheet paths
// identify a build; changing public settings does not identify a new version.
//
// Two successful observations must agree before a reload becomes eligible.
// Hidden tabs, sensitive routes, open dialogs, focused editors, and connected
// fields with unsaved input defer that reload. A bounded asset check precedes
// navigation, and a session-scoped cooldown prevents repeated reloads during
// inconsistent CDN responses. Failed probes leave the current document usable.
// @project-doc docs/operations/deployment_and_migrations.md#browser_auto_update

;(() => {
  'use strict'

  if (window.top !== window.self || window.__siteBrowserUpdater) return

  // Probe cadence and reload limits bound background traffic and protect a tab
  // from repeated navigations while a deployment or its CDN response is unstable.
  const CHECK_INTERVAL_MS = 60_000
  const TICK_INTERVAL_MS = 15_000
  const IDLE_INTERVAL_MS = 15_000
  const REQUEST_TIMEOUT_MS = 8_000
  const RELOAD_COOLDOWN_MS = 300_000
  const RELOAD_STORAGE_KEY = 'site-browser-update:last-reload:v1'
  const PROBE_PATH = '/_client-update/version'
  const PROTECTED_PATH =
    /^\/(?:auth|login|register|email-verify|forgot-password|reset-password|setup|payment|purchase|creative|ext)(?:\/|$)|^\/admin\/(?:ext(?:\/|$)|mcp\/authorize(?:\/|$))/

  // This document owns its baseline, candidate observations, and edited elements.
  // Navigation only clears edit guards after the pathname changes; closing a
  // dialog releases its guard when the edited elements leave the document.
  const baseline = readBuild(document)
  if (!baseline) return
  const editedElements = new Set()
  let currentPath = window.location.pathname
  let lastActivityAt = Date.now()
  let lastCheckedAt = 0
  let candidate = null
  let confirmations = 0
  let inFlight = false
  let reloading = false
  let suspended = false
  let controller = null
  let timer = null

  // readBuild recognizes a deployed Vite SPA and returns its resource identity.
  // Missing application markup or matching emitted module paths returns null,
  // so an error page or challenge cannot establish an application version.
  // External analytics modules, settings scripts, and HTML nonces are excluded.
  function readBuild(doc) {
    if (!doc.querySelector('#app')) return null
    const modules = Array.from(doc.querySelectorAll('script[type="module"][src]'))
    if (modules.length === 0) return null
    const paths = []
    for (const module of modules) {
      const url = new URL(module.getAttribute('src'), window.location.origin)
      if (url.origin !== window.location.origin || !url.pathname.startsWith('/assets/')) {
        // CDN analytics can append module scripts after the mount point. They
        // are absent from the parser-time baseline and cannot identify this build.
        continue
      }
      paths.push(url.pathname + url.search)
    }
    if (paths.length === 0) return null
    const entry = paths[0]
    for (const link of doc.querySelectorAll('link[rel="stylesheet"][href]')) {
      const url = new URL(link.getAttribute('href'), window.location.origin)
      if (url.origin === window.location.origin && url.pathname.startsWith('/assets/')) {
        paths.push(url.pathname + url.search)
      }
    }
    return { signature: JSON.stringify(paths.sort()), entry }
  }

  // syncRoute releases guards belonging to a page the user has left.
  // A pathname change also restarts the idle period, preventing an immediate
  // refresh while the destination component is still mounting its controls.
  function syncRoute() {
    if (currentPath === window.location.pathname) return
    currentPath = window.location.pathname
    editedElements.clear()
    lastActivityAt = Date.now()
  }

  // recordActivity restarts the idle period after the user's current interaction.
  function recordActivity() {
    syncRoute()
    lastActivityAt = Date.now()
  }

  // rememberEdit retains an edited field until it disconnects or its route changes.
  // It does not inspect field values, passwords, or tokens, and it does not infer
  // a successful save from an unrelated network response or button click.
  function rememberEdit(event) {
    recordActivity()
    if (
      event.target instanceof Element &&
      event.target.matches('input,textarea,select,[contenteditable]')
    ) {
      editedElements.add(event.target)
    }
  }

  // canReload reports whether navigation can leave the current page without an
  // observed editing or interaction guard. Authentication, checkout, creative,
  // and plugin routes remain protected until the user navigates away. Connected
  // edited fields remain guarded even after focus moves to a different control.
  function canReload() {
    syncRoute()
    if (suspended || document.visibilityState !== 'visible' || navigator.onLine === false)
      return false
    if (PROTECTED_PATH.test(currentPath) || Date.now() - lastActivityAt < IDLE_INTERVAL_MS)
      return false
    if (
      document.activeElement?.matches(
        'input,textarea,select,[contenteditable="true"],[contenteditable=""]',
      )
    ) {
      return false
    }
    for (const element of document.querySelectorAll(
      'dialog[open],[role="dialog"],[aria-modal="true"],[aria-busy="true"],[data-browser-update-block]',
    )) {
      if (element.getClientRects().length > 0) return false
    }
    for (const element of editedElements) {
      if (element.isConnected) return false
      editedElements.delete(element)
    }
    return true
  }

  // reloadWhenSafe verifies the candidate entry and records a cooldown before
  // navigating. Call it only from the serialized check while its abort signal is
  // alive. Conditions are checked again after the asset request because a user
  // can start editing or change routes while that independent request is pending.
  // Storage must retain the receipt before reload is allowed, preventing a tab
  // with disabled session storage from entering an unbounded refresh cycle.
  async function reloadWhenSafe(signal) {
    if (!canReload()) return
    const previous = Number(window.sessionStorage.getItem(RELOAD_STORAGE_KEY) || 0)
    if (!Number.isFinite(previous) || Date.now() - previous < RELOAD_COOLDOWN_MS) return
    const asset = await fetch(candidate.entry, {
      method: 'HEAD',
      cache: 'no-store',
      credentials: 'same-origin',
      redirect: 'error',
      signal,
    })
    if (!asset.ok || !/(?:javascript|ecmascript)/i.test(asset.headers.get('content-type') || ''))
      return
    if (!canReload()) return
    // Persist before navigation: the next document reads this receipt even when
    // a CDN briefly serves the previous build again after the reload completes.
    const receipt = String(Date.now())
    window.sessionStorage.setItem(RELOAD_STORAGE_KEY, receipt)
    if (window.sessionStorage.getItem(RELOAD_STORAGE_KEY) !== receipt) return
    reloading = true
    window.location.reload()
  }

  // check observes a fresh origin document without overlapping another probe.
  // The regular cadence is one minute; a pending candidate is confirmed on the
  // next fifteen-second tick. Two matching successful observations establish a
  // candidate, while malformed documents and failures discard confirmation.
  //
  // One abort controller bounds the HTML and optional asset requests together.
  // Only the owning invocation clears the request state in finally. A reload
  // requires an available entry, an idle eligible page, and a stored cooldown;
  // probe or storage failures leave the existing application running normally.
  async function check(force = false) {
    syncRoute()
    if (
      suspended ||
      reloading ||
      inFlight ||
      document.visibilityState !== 'visible' ||
      navigator.onLine === false
    )
      return
    const interval = candidate ? TICK_INTERVAL_MS : CHECK_INTERVAL_MS
    if (Date.now() - lastCheckedAt < (force ? TICK_INTERVAL_MS : interval)) return
    inFlight = true
    lastCheckedAt = Date.now()
    controller = new AbortController()
    const timeout = window.setTimeout(() => controller?.abort(), REQUEST_TIMEOUT_MS)
    try {
      const url = new URL(PROBE_PATH, window.location.origin)
      url.searchParams.set('_t', String(lastCheckedAt))
      const response = await fetch(url.href, {
        cache: 'no-store',
        credentials: 'same-origin',
        redirect: 'error',
        headers: { Accept: 'text/html' },
        signal: controller.signal,
      })
      if (!response.ok || !/text\/html/i.test(response.headers.get('content-type') || '')) {
        throw new Error('Frontend version probe is unavailable')
      }
      const fresh = readBuild(new DOMParser().parseFromString(await response.text(), 'text/html'))
      if (!fresh) throw new Error('Frontend version probe is not an application document')
      document.documentElement.dataset.browserAutoUpdateLastCheck = String(lastCheckedAt)
      if (fresh.signature === baseline.signature) {
        candidate = null
        confirmations = 0
        return
      }
      // Require consecutive agreement rather than trusting one response from a
      // deployment whose proxy or edge nodes may still serve different builds.
      confirmations = candidate?.signature === fresh.signature ? confirmations + 1 : 1
      candidate = fresh
      if (confirmations >= 2) await reloadWhenSafe(controller.signal)
    } catch {
      // Version detection is optional. Discard uncertain observations after any
      // failure instead of turning a network outage into a page reload.
      candidate = null
      confirmations = 0
    } finally {
      window.clearTimeout(timeout)
      controller = null
      inFlight = false
    }
  }

  // pause stops document-owned polling when navigation hides this page.
  // Aborting the current request prevents a late response from refreshing a
  // document that is entering the browser's back-forward cache.
  function pause() {
    suspended = true
    window.clearInterval(timer)
    timer = null
    controller?.abort()
  }

  // resume restarts polling when a page returns from the back-forward cache.
  // The interval is registered at most once, and resumed pages receive a fresh
  // idle period before any confirmed candidate may navigate the current tab.
  function resume() {
    suspended = false
    recordActivity()
    if (timer === null) timer = window.setInterval(() => void check(), TICK_INTERVAL_MS)
    void check(true)
  }

  window.__siteBrowserUpdater = {
    check: () => check(true),
    status: () => ({ pending: confirmations >= 2, lastCheckedAt, reloading }),
  }
  document.documentElement.dataset.browserAutoUpdate = 'ready'
  for (const event of ['pointerdown', 'keydown', 'wheel', 'touchstart']) {
    document.addEventListener(event, recordActivity, {
      capture: true,
      passive: true,
    })
  }
  document.addEventListener('input', rememberEdit, true)
  document.addEventListener('change', rememberEdit, true)
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
      recordActivity()
      void check(true)
    }
  })
  window.addEventListener('focus', () => {
    recordActivity()
    void check(true)
  })
  window.addEventListener('online', () => void check(true))
  window.addEventListener('vite:preloadError', () => void check(true))
  window.addEventListener('pagehide', pause)
  window.addEventListener('pageshow', resume)
  timer = window.setInterval(() => void check(), TICK_INTERVAL_MS)
})()
