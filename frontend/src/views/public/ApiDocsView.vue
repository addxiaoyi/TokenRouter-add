<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppHeader from '@/components/layout/AppHeader.vue'
import Icon from '@/components/icons/Icon.vue'
import { buildGatewayUrl } from '@/api/url'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'
import { sanitizeUrl } from '@/utils/url'

// ApiDocsView.vue gives developers a runnable path from an API key to a first
// gateway request. Examples use the deployment's gateway origin; the response
// to model discovery determines which model IDs the key can use. The guide
// remains readable without a session or a successful public settings request.

// The guide uses browser language and gateway configuration without loading any
// personal credentials; configured external documentation stays available separately.
const { t } = useI18n()
const app = useAppStore()
const { copyToClipboard } = useClipboard()
const baseUrl = buildGatewayUrl('/v1')
const externalDocs = computed(() => sanitizeUrl(app.docUrl))

// The examples follow the client workflow from listing available models to
// choosing a protocol and retrieving the result using the original key.
const examples = [
  { id: 'models', method: 'GET', path: '/models' },
  {
    id: 'chat',
    method: 'POST',
    path: '/chat/completions',
    body: { model: 'MODEL_ID', messages: [{ role: 'user', content: 'Hello' }] },
  },
  {
    id: 'responses',
    method: 'POST',
    path: '/responses',
    body: { model: 'MODEL_ID', input: 'Hello' },
  },
  {
    id: 'embeddings',
    method: 'POST',
    path: '/embeddings',
    body: { model: 'EMBEDDING_MODEL_ID', input: 'Text to embed' },
  },
  {
    id: 'images',
    method: 'POST',
    path: '/images/generations',
    body: { model: 'IMAGE_MODEL_ID', prompt: 'A mountain lake at dawn' },
  },
  {
    id: 'videos',
    method: 'POST',
    path: '/videos/generations',
    body: { model: 'VIDEO_MODEL_ID', prompt: 'A slow aerial shot over a mountain lake' },
  },
  { id: 'videoStatus', method: 'GET', path: '/videos/generations/REQUEST_ID' },
].map(({ id, method, path, body }) => {
  const lines = [`curl --fail-with-body '${baseUrl}${path}'`, `-H "Authorization: Bearer $API_KEY"`]
  if (body) lines.push(`-H 'Content-Type: application/json'`, `-d '${JSON.stringify(body)}'`)
  return { id, method, path, command: lines.join(' ' + '\\' + '\n  ') }
})

// These codes describe gateway errors that callers commonly need to diagnose
// before changing credentials, request arguments, or retry behavior.
const errorCodes = [400, 401, 403, 404, 429, 502]

// copyExample copies the selected public configuration value without reading an
// API key. Clipboard failures leave selectable text in place and show feedback
// even if the fallback clipboard command throws.
async function copyExample(event: MouseEvent) {
  if (!(event.currentTarget instanceof HTMLButtonElement)) return
  try {
    await copyToClipboard(event.currentTarget.value)
  } catch {
    app.showError(t('common.copyFailed'))
  }
}
</script>

<template>
  <div class="ba-theme-shell relative min-h-screen pt-[var(--header-h)]">
    <div class="ba-theme-backdrop pointer-events-none fixed inset-0" aria-hidden="true"></div>
    <AppHeader public-page />

    <main class="relative z-10 mx-auto w-full max-w-6xl px-4 py-8 sm:px-6 sm:py-10">
      <header class="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div class="page-heading min-w-0">
          <h1 class="page-title">{{ t('apiDocs.title') }}</h1>
          <p class="page-description">{{ t('apiDocs.intro') }}</p>
        </div>
        <RouterLink to="/keys" class="btn btn-primary">
          <Icon name="key" size="sm" :animate-on-hover="false" />
          {{ t('apiDocs.manageKeys') }}
        </RouterLink>
      </header>

      <div class="grid min-w-0 items-start gap-4 lg:grid-cols-[14rem_minmax(0,1fr)]">
        <nav
          :aria-label="t('apiDocs.sections')"
          class="card flex flex-wrap gap-2 p-4 lg:sticky lg:top-[calc(var(--header-h)+1rem)] lg:flex-col"
        >
          <a href="#connect" class="btn btn-secondary btn-sm lg:justify-start">
            {{ t('apiDocs.connect') }}
          </a>
          <a href="#examples" class="btn btn-secondary btn-sm lg:justify-start">
            {{ t('apiDocs.examples') }}
          </a>
          <a href="#errors" class="btn btn-secondary btn-sm lg:justify-start">
            {{ t('apiDocs.errors') }}
          </a>
          <RouterLink to="/models" class="btn btn-secondary btn-sm lg:justify-start">
            {{ t('nav.modelMarketplace') }}
          </RouterLink>
          <a
            v-if="externalDocs"
            :href="externalDocs"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary btn-sm lg:justify-start"
          >
            {{ t('apiDocs.externalDocs') }}
            <Icon name="externalLink" size="xs" :animate-on-hover="false" />
          </a>
        </nav>

        <div class="min-w-0 space-y-4">
          <section
            id="connect"
            aria-labelledby="connect-title"
            class="card scroll-mt-24 space-y-4 p-4 sm:p-6"
          >
            <h2 id="connect-title" class="text-lg font-semibold">{{ t('apiDocs.connect') }}</h2>
            <div
              class="flex flex-wrap items-center justify-between gap-2 rounded-control bg-gray-50 p-4 dark:bg-dark-900"
            >
              <div class="min-w-0 space-y-1">
                <div class="text-sm font-medium">{{ t('apiDocs.baseUrl') }}</div>
                <code class="block break-all font-mono text-sm">{{ baseUrl }}</code>
              </div>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :value="baseUrl"
                @click="copyExample"
              >
                <Icon name="copy" size="sm" :animate-on-hover="false" />
                {{ t('apiDocs.copyUrl') }}
              </button>
            </div>
            <ol class="list-decimal space-y-2 pl-5 text-sm text-gray-600 dark:text-dark-300">
              <li>{{ t('apiDocs.stepKey') }}</li>
              <li>{{ t('apiDocs.stepModel') }}</li>
              <li>{{ t('apiDocs.stepUsage') }}</li>
            </ol>
          </section>

          <section
            id="examples"
            aria-labelledby="examples-title"
            class="card min-w-0 scroll-mt-24 p-4 sm:p-6"
          >
            <h2 id="examples-title" class="text-lg font-semibold">{{ t('apiDocs.examples') }}</h2>
            <p class="mt-2 text-sm text-gray-600 dark:text-dark-300">{{ t('apiDocs.shell') }}</p>
            <div class="mt-4 divide-y divide-gray-200 dark:divide-dark-700">
              <article
                v-for="example in examples"
                :key="example.id"
                class="min-w-0 space-y-2 py-4 last:pb-0"
              >
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="min-w-0">
                    <h3 class="font-medium">{{ t('apiDocs.endpoint.' + example.id) }}</h3>
                    <code class="break-all font-mono text-sm text-gray-600 dark:text-dark-300">
                      {{ example.method }} /v1{{ example.path }}
                    </code>
                  </div>
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm"
                    :value="example.command"
                    :aria-label="
                      t('apiDocs.copyNamed', { name: t('apiDocs.endpoint.' + example.id) })
                    "
                    @click="copyExample"
                  >
                    <Icon name="copy" size="sm" :animate-on-hover="false" />
                    {{ t('apiDocs.copyCommand') }}
                  </button>
                </div>
                <p
                  v-if="example.id === 'videoStatus'"
                  class="text-sm text-gray-600 dark:text-dark-300"
                >
                  {{ t('apiDocs.pollVideo') }}
                </p>
                <pre
                  class="code-block max-w-full whitespace-pre-wrap break-all"
                ><code>{{ example.command }}</code></pre>
              </article>
            </div>
          </section>

          <section
            id="errors"
            aria-labelledby="errors-title"
            class="card scroll-mt-24 space-y-4 p-4 sm:p-6"
          >
            <h2 id="errors-title" class="text-lg font-semibold">{{ t('apiDocs.errors') }}</h2>
            <dl class="divide-y divide-gray-200 dark:divide-dark-700">
              <div
                v-for="code in errorCodes"
                :key="code"
                class="grid grid-cols-[3rem_minmax(0,1fr)] gap-2 py-2 text-sm"
              >
                <dt class="font-mono font-semibold">{{ code }}</dt>
                <dd class="text-gray-600 dark:text-dark-300">{{ t('apiDocs.error.' + code) }}</dd>
              </div>
            </dl>
            <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('apiDocs.troubleshoot') }}</p>
          </section>
        </div>
      </div>
    </main>
  </div>
</template>
