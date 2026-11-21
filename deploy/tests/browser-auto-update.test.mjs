import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import test from 'node:test'
import vm from 'node:vm'

// browser-auto-update.test.mjs exercises deployed-version detection with real
// jsdom parsing and deterministic browser clocks. Network responses, navigation,
// and timer scheduling remain isolated from the live website. The cases protect
// idle refresh, editing and checkout guards, uncertain responses, and cooldowns.

const require = createRequire(new URL('../../frontend/package.json', import.meta.url))
const { JSDOM } = require('jsdom')
const source = readFileSync(new URL('../browser-auto-update.js', import.meta.url), 'utf8')

// html supplies a Vite-shaped application document with independently varied CSS.
function html(version, css = version) {
  return `<html><head><script type="module" src="/assets/index-${version}.js"></script><link rel="stylesheet" href="/assets/index-${css}.css"></head><body><div id="app"></div></body></html>`
}

// response supplies the status, MIME metadata, and body consumed by browser probes.
function response(body, type = 'text/html', ok = true) {
  return {
    ok,
    headers: new Headers({ 'content-type': type }),
    text: async () => body,
  }
}

// harness runs the shipped script with a real DOM and replaceable I/O boundaries.
// The caller advances time before invoking checks; scheduled callbacks are kept
// inert until explicitly exercised. Cleanup closes jsdom without touching network
// services, while reload records the original URL instead of navigating anywhere.
function harness(t) {
  const dom = new JSDOM(html('old'), {
    url: 'https://site.example/usage?range=week#details',
    pretendToBeVisual: true,
  })
  t.after(() => dom.window.close())
  let clock = 1_800_000_000_000
  let sequence = 0
  let visibility = 'visible'
  const intervals = new Map()
  const timeouts = new Map()
  const requests = []
  const reloads = []
  const navigator = { onLine: true }
  const location = {
    origin: 'https://site.example',
    pathname: '/usage',
    href: 'https://site.example/usage?range=week#details',
    reload: () => reloads.push(location.href),
  }
  const network = {
    html: () => response(html('new')),
    asset: () => response('', 'application/javascript'),
  }
  Object.defineProperty(dom.window.document, 'visibilityState', {
    get: () => visibility,
  })
  const browser = {
    location,
    sessionStorage: dom.window.sessionStorage,
    addEventListener: dom.window.addEventListener.bind(dom.window),
    setTimeout: (callback) => {
      timeouts.set(++sequence, callback)
      return sequence
    },
    clearTimeout: (id) => timeouts.delete(id),
    setInterval: (callback) => {
      intervals.set(++sequence, callback)
      return sequence
    },
    clearInterval: (id) => intervals.delete(id),
  }
  browser.top = browser
  browser.self = browser
  const context = vm.createContext({
    window: browser,
    document: dom.window.document,
    navigator,
    Element: dom.window.Element,
    DOMParser: dom.window.DOMParser,
    URL,
    AbortController,
    Date: class extends Date {
      // now returns the harness clock without advancing real browser time.
      static now() {
        return clock
      }
    },
    fetch: async (url, init) => {
      requests.push({ url: String(url), init })
      return init.method === 'HEAD' ? network.asset(init) : network.html(init)
    },
  })
  vm.runInContext(source, context)
  return {
    dom,
    browser,
    context,
    navigator,
    network,
    requests,
    reloads,
    intervals,
    timeouts,
    advance: (milliseconds = 60_000) => {
      clock += milliseconds
    },
    visible: (value) => {
      visibility = value
    },
    check: () => browser.__siteBrowserUpdater.check(),
    event: (name) => dom.window.dispatchEvent(new dom.window.Event(name)),
    edit: (element) => element.dispatchEvent(new dom.window.Event('input', { bubbles: true })),
  }
}

// Identical resource paths survive changing public configuration and HTML nonces.
// The watcher sends uncached HTML probes without adding credentials to request headers.
test('same deployed build does not reload', async (t) => {
  const h = harness(t)
  h.network.html = () =>
    response(
      html('old').replace('</head>', '<script nonce="different">window.config={}</script></head>'),
    )
  h.advance()
  await h.check()
  h.advance()
  await h.check()
  assert.equal(h.reloads.length, 0)
  assert.equal(h.requests.length, 2)
  assert.equal(h.requests[0].init.cache, 'no-store')
  assert.equal(h.requests[0].init.redirect, 'error')
  assert.deepEqual(h.requests[0].init.headers.Accept, 'text/html')
  assert.equal(h.requests[0].init.headers.Authorization, undefined)
})

// A new deployment appears twice while the user remains idle on a reading page.
// Reload preserves the full URL and existing browser storage after checking the new entry.
test('confirmed idle update reloads once and keeps URL and login storage', async (t) => {
  const h = harness(t)
  h.browser.sessionStorage.setItem('existing-session', 'retained')
  h.advance()
  await h.check()
  assert.equal(h.reloads.length, 0)
  h.advance(15_000)
  await h.check()
  assert.deepEqual(h.reloads, ['https://site.example/usage?range=week#details'])
  assert.equal(h.browser.sessionStorage.getItem('existing-session'), 'retained')
  assert.equal(h.requests.at(-1).init.method, 'HEAD')
  assert.equal(h.requests.at(-1).url, '/assets/index-new.js')
  h.advance()
  await h.check()
  assert.equal(h.reloads.length, 1)
})

// Different edge nodes temporarily return different resources during a deployment.
// A baseline response cancels the candidate, and subsequent matching observations are required again.
test('mixed deployment responses do not trigger a single-observation reload', async (t) => {
  const h = harness(t)
  h.advance()
  await h.check()
  h.network.html = () => response(html('old'))
  h.advance(15_000)
  await h.check()
  h.network.html = () => response(html('newer'))
  h.advance()
  await h.check()
  assert.equal(h.reloads.length, 0)
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 1)
})

// A background or offline tab cannot validate the currently deployed application.
// Polling stays silent in both states and resumes only after the tab becomes eligible.
test('hidden and offline tabs defer all probes', async (t) => {
  const h = harness(t)
  h.advance()
  h.visible('hidden')
  await h.check()
  h.visible('visible')
  h.navigator.onLine = false
  await h.check()
  assert.equal(h.requests.length, 0)
  h.navigator.onLine = true
  await h.check()
  assert.equal(h.requests.length, 1)
})

// Authentication, payment, creation, and plugin pages may own state outside ordinary forms.
// Each protected route retains its document even after a new build is confirmed twice.
test('sensitive routes never refresh automatically', async (t) => {
  for (const path of [
    '/login',
    '/register',
    '/email-verify',
    '/forgot-password',
    '/reset-password',
    '/auth/callback',
    '/setup',
    '/payment/qrcode',
    '/payment/result',
    '/purchase',
    '/creative',
    '/ext/example',
    '/admin/ext/example',
    '/admin/mcp/authorize',
  ]) {
    const h = harness(t)
    h.browser.location.pathname = path
    h.advance()
    await h.check()
    h.advance(15_000)
    await h.check()
    assert.equal(h.reloads.length, 0, path)
  }
})

// A form remains connected after the user types and moves focus elsewhere.
// Closing its containing editor removes the guard and allows the confirmed update later.
test('edited connected fields defer update until their editor is removed', async (t) => {
  const h = harness(t)
  const input = h.dom.window.document.createElement('input')
  h.dom.window.document.body.append(input)
  h.edit(input)
  h.advance()
  await h.check()
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 0)
  input.remove()
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 1)
})

// Leaving an edited page invalidates its edit guard but starts a fresh idle period.
// The destination is not refreshed while navigation has just mounted its replacement content.
test('route changes release old edits after a new idle interval', async (t) => {
  const h = harness(t)
  const input = h.dom.window.document.createElement('input')
  h.dom.window.document.body.append(input)
  h.edit(input)
  h.advance()
  await h.check()
  h.advance(15_000)
  await h.check()
  input.remove()
  h.browser.location.pathname = '/dashboard'
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 0)
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 1)
})

// Focus and a visible modal independently protect work even without input events.
// Removing both protections allows the already confirmed deployment to load on a later check.
test('focused editors and visible dialogs delay reload', async (t) => {
  const h = harness(t)
  const input = h.dom.window.document.createElement('input')
  const dialog = h.dom.window.document.createElement('div')
  dialog.setAttribute('role', 'dialog')
  dialog.getClientRects = () => [{}]
  h.dom.window.document.body.append(input, dialog)
  input.focus()
  h.advance()
  await h.check()
  h.advance(15_000)
  await h.check()
  input.blur()
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 0)
  dialog.remove()
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 1)
})

// Network failures, error pages, and foreign module entries provide no trustworthy version.
// Each failure clears prior confirmation rather than using stale evidence to navigate away.
test('unavailable or invalid probe responses keep the existing document', async (t) => {
  const failures = [
    () => {
      throw new Error('offline')
    },
    () => response(html('new'), 'text/html', false),
    () => response('{}', 'application/json'),
    () => response('<html>Verify you are human</html>'),
    () =>
      response(
        html('new').replace('/assets/index-new.js', 'https://other.example/assets/index-new.js'),
      ),
  ]
  for (const failure of failures) {
    const h = harness(t)
    h.advance()
    await h.check()
    h.network.html = failure
    h.advance(15_000)
    await h.check()
    assert.equal(h.reloads.length, 0)
    assert.equal(h.browser.__siteBrowserUpdater.status().pending, false)
  }
})

// A newly advertised entry may be missing while deployment assets are being replaced.
// A failed HEAD or an HTML fallback prevents navigation until the JavaScript entry is available.
test('missing or non-JavaScript candidate entries cannot trigger reload', async (t) => {
  const h = harness(t)
  h.network.asset = () => response('', 'text/html')
  h.advance()
  await h.check()
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 0)
  h.network.asset = () => response('', 'application/javascript', false)
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 0)
  h.network.asset = () => response('', 'text/javascript')
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 1)
})

// A previous automatic refresh leaves a tab-scoped timestamp for the next document.
// Repeated mismatches cannot refresh that tab again until the five-minute cooldown has elapsed.
test('session cooldown and disabled storage prevent reload loops', async (t) => {
  const h = harness(t)
  h.browser.sessionStorage.setItem('site-browser-update:last-reload:v1', '1800000000000')
  h.advance()
  await h.check()
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 0)
  h.advance(300_000)
  await h.check()
  assert.equal(h.reloads.length, 1)
  const disabled = harness(t)
  disabled.browser.sessionStorage = {
    getItem: () => null,
    setItem: () => {
      throw new Error('blocked')
    },
  }
  disabled.advance()
  await disabled.check()
  disabled.advance(15_000)
  await disabled.check()
  assert.equal(disabled.reloads.length, 0)
})

// A reload attempt can outlive a user beginning a new form interaction.
// Rechecking guards after the asset response prevents that late response from discarding the new edit.
test('editing during candidate asset validation cancels navigation', async (t) => {
  const h = harness(t)
  let release
  h.network.asset = () =>
    new Promise((resolve) => {
      release = resolve
    })
  h.advance()
  await h.check()
  h.advance(15_000)
  const checking = h.check()
  await new Promise((resolve) => setImmediate(resolve))
  const input = h.dom.window.document.createElement('input')
  h.dom.window.document.body.append(input)
  h.edit(input)
  release(response('', 'application/javascript'))
  await checking
  assert.equal(h.reloads.length, 0)
})

// Duplicate injection and back-forward caching must not accumulate polling intervals.
// A pending request is aborted on pagehide, then pageshow restores exactly one interval.
test('polling is singleton, serialized, and suspended across page navigation', async (t) => {
  const h = harness(t)
  vm.runInContext(source, h.context)
  assert.equal(h.intervals.size, 1)
  h.network.html = (init) =>
    new Promise((resolve, reject) => {
      init.signal.addEventListener('abort', () => reject(new Error('aborted')), { once: true })
    })
  h.advance()
  const pending = h.check()
  await h.check()
  assert.equal(h.requests.length, 1)
  h.event('pagehide')
  await pending
  assert.equal(h.intervals.size, 0)
  assert.equal(h.timeouts.size, 0)
  h.event('pageshow')
  h.event('pageshow')
  assert.equal(h.intervals.size, 1)
  assert.equal(h.reloads.length, 0)
})

// A deployment may replace only styles while leaving the module entry path unchanged.
// Its stylesheet fingerprint still triggers an idle refresh after two matching observations.
test('stylesheet-only deployments are detected', async (t) => {
  const h = harness(t)
  h.network.html = () => response(html('old', 'new-css'))
  h.advance()
  await h.check()
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 1)
})

// A probe that never receives headers must release its single-flight slot.
// The bounded timeout aborts that request and a later healthy request can proceed.
test('timed out probes release their request slot', async (t) => {
  const h = harness(t)
  h.network.html = (init) =>
    new Promise((resolve, reject) => {
      init.signal.addEventListener('abort', () => reject(new Error('timeout')), { once: true })
    })
  h.advance()
  const pending = h.check()
  for (const abort of h.timeouts.values()) abort()
  await pending
  assert.equal(h.timeouts.size, 0)
  assert.equal(h.reloads.length, 0)
  h.network.html = () => response(html('old'))
  h.advance()
  await h.check()
  assert.equal(h.requests.length, 2)
  assert.equal(h.dom.window.document.documentElement.dataset.browserAutoUpdate, 'ready')
  assert.ok(Number(h.dom.window.document.documentElement.dataset.browserAutoUpdateLastCheck) > 0)
})

// CDN analytics can append an external module after the application mount point.
// That module neither invalidates a legitimate probe nor changes the recognized application build.
test('CDN analytics modules are excluded from deployed version identity', async (t) => {
  const h = harness(t)
  const analytics =
    '<script type="module" src="https://static.cloudflareinsights.com/beacon.js"></script>'
  h.network.html = () => response(html('old').replace('</body>', analytics + '</body>'))
  h.advance()
  await h.check()
  assert.ok(Number(h.dom.window.document.documentElement.dataset.browserAutoUpdateLastCheck) > 0)
  assert.equal(h.reloads.length, 0)
  h.network.html = () => response(html('new').replace('</body>', analytics + '</body>'))
  h.advance()
  await h.check()
  h.advance(15_000)
  await h.check()
  assert.equal(h.reloads.length, 1)
})
