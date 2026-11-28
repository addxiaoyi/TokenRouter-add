import { defineConfig, loadEnv, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import checker from 'vite-plugin-checker'
import { resolve } from 'path'
// Native config loading requires source extensions; tsconfig.node.json rewrites
// this extension so emitted JavaScript resolves the compiled metadata module.
import { renderSocialMetadata, resolvePageMetadata } from './socialMetadata.ts'

// vite.config.ts configures development proxying and the browser assets embedded
// in release builds. Development documents receive public settings and branding
// before application startup. Vendor groups isolate feature libraries so shared
// startup dependencies cannot pull editors, media tools, or payment SDKs into
// the initial download. Unclassified modules retain automatic import boundaries,
// while strict execution order preserves initialization across generated chunks.

// escapeHtml encodes HTML-sensitive characters in injected branding text and attributes.
function escapeHtml(value: string): string {
  return value.replace(
    /[&<>"']/g,
    (character) =>
      ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
      })[character] || character,
  )
}

// isSafeImageUrl recognizes the source syntax allowed for an injected site icon.
// Root-relative paths, image data URLs, and HTTP(S) URLs pass this filter. Callers
// use it for source selection; resource contents are outside this syntax check.
function isSafeImageUrl(value: string): boolean {
  const trimmed = value.trim()
  if ((trimmed.startsWith('/') && !trimmed.startsWith('//')) || /^data:image\//i.test(trimmed)) {
    return true
  }
  try {
    const parsed = new URL(trimmed)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

// injectBranding replaces the existing title with the resolved route title and updates
// the icon from site settings. Values are escaped before insertion, and icon replacement
// requires an allowed source URL. Missing template tags remain absent from the result.
function injectBranding(
  html: string,
  config: {
    site_logo?: string // Optional favicon source URL.
  },
  documentTitle: string,
): string {
  let brandedHtml = html
  brandedHtml = brandedHtml.replace(
    /<title>[^<]*<\/title>/i,
    `<title>${escapeHtml(documentTitle)}</title>`,
  )

  const siteLogo = config.site_logo?.trim()
  if (siteLogo && isSafeImageUrl(siteLogo)) {
    brandedHtml = brandedHtml.replace(
      /<link\s+rel=["']icon["'][^>]*>/i,
      `<link rel="icon" href="${escapeHtml(siteLogo)}" />`,
    )
  }
  return brandedHtml
}

// injectPublicSettings creates the development document-transform plugin for public
// settings. Its pre-transform completes before the app reads injected branding and
// access flags. Unsuccessful or unavailable configuration leaves the template intact
// so browser startup can request public settings through its normal API path.
// devOrigin supplies the local sharing destination when frontend_url is blank.
function injectPublicSettings(backendUrl: string, devOrigin: string): Plugin {
  return {
    name: 'inject-public-settings',
    apply: 'serve',
    transformIndexHtml: {
      order: 'pre',
      // handler fetches startup configuration for one development document render.
      // Only a successful API envelope is injected. A request timeout, network
      // failure, or transform error leaves the original document for client loading.
      async handler(html, context) {
        try {
          // Bound configuration loading to two seconds so a disconnected backend
          // does not indefinitely delay serving the browser's application document.
          const response = await fetch(`${backendUrl}/api/v1/settings/public`, {
            signal: AbortSignal.timeout(2000),
          })
          if (response.ok) {
            const data = await response.json()
            if (data.code === 0 && data.data) {
              const script = `<script>window.__APP_CONFIG__=${JSON.stringify(data.data)};</script>`
              const requestURL = new URL(context.path, devOrigin).href
              const metadata = renderSocialMetadata(data.data, requestURL)
              // Replace the static description before inserting route-specific metadata so crawlers and browsers see one current value.
              const branded = injectBranding(
                html,
                data.data,
                resolvePageMetadata(data.data, requestURL).title,
              ).replace(/<meta\b[^>]*\bname=["']description["'][^>]*>/gi, '')
              return branded.replace('</head>', `${metadata}\n${script}\n</head>`)
            }
          }
        } catch (e) {
          // The browser can fetch public settings after startup, so a failed
          // development-time injection is logged and returns the unmodified template.
          console.warn(
            '[vite] Failed to fetch public settings, the browser will request them through the API:',
            (e as Error).message,
          )
        }
        return html
      },
    },
  }
}

export default defineConfig(({ mode }) => {
  // Development addresses come from the selected mode environment, with local server defaults for an unconfigured checkout.
  const env = loadEnv(mode, process.cwd(), '')
  const backendUrl = env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080'
  const devPort = Number(env.VITE_DEV_PORT || 3000)

  return {
    plugins: [
      vue(),
      checker({
        vueTsc: true,
      }),
      injectPublicSettings(backendUrl, `http://localhost:${devPort}`),
    ],
    resolve: {
      alias: {
        '@': resolve(import.meta.dirname, 'src'),
        // Host code consumes the SDK's shared error class and constants directly;
        // published plugin bundles still externalize the full SDK to host globals.
        '@tokenflux/tokenrouter-plugin-sdk': resolve(
          import.meta.dirname,
          '../plugin-sdk/web/src/types.ts',
        ),
        // Vue I18n 11 enables CSP-compatible JIT compilation by default; keep
        // the runtime distribution used by the application and component tests.
        'vue-i18n': 'vue-i18n/dist/vue-i18n.runtime.esm-bundler.js',
      },
    },
    build: {
      outDir: '../backend/internal/web/dist',
      emptyOutDir: true,
      rolldownOptions: {
        preserveEntrySignatures: false,
        output: {
          strictExecutionOrder: true,
          codeSplitting: {
            groups: [
              {
                test: /node_modules[\\/]/,
                debugName: 'feature-vendors',
                includeDependenciesRecursively: false,
                // name groups known libraries by runtime responsibility and leaves
                // other modules to automatic splitting. A catch-all vendor group
                // would make every startup consumer download unrelated lazy features.
                name(id: string) {
                  const moduleId = id.replaceAll('\\', '/')
                  if (moduleId.includes('node_modules')) {
                    // Vue runtime, routing, state, and compiler packages share this vendor group.
                    if (
                      moduleId.includes('/vue/') ||
                      moduleId.includes('/vue-router/') ||
                      moduleId.includes('/pinia/') ||
                      moduleId.includes('/@vue/')
                    ) {
                      return 'vendor-vue'
                    }

                    // Reused browser composition utilities share the UI vendor group.
                    if (moduleId.includes('/@vueuse/')) {
                      return 'vendor-ui'
                    }

                    // Chart rendering libraries share a group used by analytics pages.
                    if (moduleId.includes('/chart.js/') || moduleId.includes('/vue-chartjs/')) {
                      return 'vendor-chart'
                    }

                    // Localization packages remain together with their message compiler dependencies.
                    if (moduleId.includes('/vue-i18n/') || moduleId.includes('/@intlify/')) {
                      return 'vendor-i18n'
                    }

                    // Stripe initialization follows its dynamically imported payment entry point.
                    if (moduleId.includes('/@stripe/stripe-js/')) {
                      return 'vendor-stripe'
                    }

                    // Airwallex prefetches payment scripts during initialization; keeping its
                    // namespace separate preserves the payment page as that load trigger.
                    if (moduleId.includes('/@airwallex/')) {
                      return 'vendor-airwallex'
                    }

                    // Editors and visual effects load independently of account pages;
                    // each package keeps its own lazy consumer as the download trigger.
                    if (moduleId.includes('/fabric/')) return 'vendor-canvas'
                    if (moduleId.includes('/cropperjs/') || moduleId.includes('/@cropper/')) {
                      return 'vendor-cropper'
                    }
                    if (
                      moduleId.includes('/gifuct-js/') ||
                      moduleId.includes('/js-binary-schema-parser/')
                    ) {
                      return 'vendor-gif'
                    }
                    if (moduleId.includes('/matter-js/')) return 'vendor-physics'
                    if (moduleId.includes('/ogl/')) return 'vendor-webgl'
                    if (moduleId.includes('/lottie-web/')) return 'vendor-lottie'

                    // Content parsing, tours, and exports stay out of shared HTTP
                    // dependencies even when their consumers use the same application stores.
                    if (moduleId.includes('/marked/')) return 'vendor-markdown'
                    if (moduleId.includes('/driver.js/')) return 'vendor-tour'
                    if (moduleId.includes('/qrcode/')) return 'vendor-qrcode'
                    if (moduleId.includes('/xlsx/')) return 'vendor-spreadsheet'
                  }

                  return null
                },
              },
            ],
          },
        },
      },
    },
    server: {
      host: '0.0.0.0',
      port: devPort,
      proxy: {
        '/mcp': {
          target: backendUrl,
          changeOrigin: true,
        },
        '/api': {
          target: backendUrl,
          changeOrigin: true,
        },
        '/v1': {
          target: backendUrl,
          changeOrigin: true,
        },
        '/setup': {
          target: backendUrl,
          changeOrigin: true,
        },
      },
    },
  }
})
