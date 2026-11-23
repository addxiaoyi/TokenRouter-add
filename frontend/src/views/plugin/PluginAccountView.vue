<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import { usePluginsStore } from '@/stores/plugins'
import { useAuthStore } from '@/stores/auth'
import { accountPages, availableAccountPanels } from '@/utils/pluginAccountPages'
import PluginPageContent from './PluginPageContent.vue'

// PluginAccountView.vue presents related user contributions on one account page.
// Each available panel mounts through the ordinary plugin content host, preserving
// exact audience checks, accepted bundles, and plugin capability scopes. Navigation
// uses host-owned labels while the installed components keep their existing forms.
//
// Tabs hide mounted panels instead of replacing them so switching between personal
// and enterprise verification preserves unsaved entries. Account changes replace
// the panel group, and each content host independently replaces its scope after
// discovery changes. Discovery failures expose a retry instead of an empty account
// page, and missing contributions produce an explicit unavailable state.

// Route selection and discovery jointly determine the authorized visible panels;
// retry state belongs to this page and does not replace installed plugin settings.
const route = useRoute()
const store = usePluginsStore()
const auth = useAuthStore()
const { t } = useI18n()
const failed = ref(false)
const loading = ref(true)
const page = computed(() => accountPages.find((entry) => entry.path === route.path))
const panels = computed(() => (page.value ? availableAccountPanels(page.value, store.plugins) : []))
const active = computed(
  () => panels.value.find((panel) => panel.id === route.query.tab)?.id ?? panels.value[0]?.id,
)

// discover reads current user contributions and presents failures with a retry.
// Bundle loading remains inside each content host. Repeating discovery is safe
// because the store coalesces requests and filters descriptions by authenticated scope.
async function discover(): Promise<void> {
  failed.value = false
  loading.value = true
  try {
    await store.ensureUi()
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
}

void discover()
</script>

<template>
  <AppLayout :hide-page-heading="true">
    <section class="space-y-4">
      <h1 class="text-xl font-semibold">{{ t(page?.titleKey ?? 'plugins.pageTitle') }}</h1>
      <ContentSkeleton v-if="loading" :rows="3" />
      <div v-else-if="failed" class="card p-5" role="alert">
        <p>{{ t('plugins.loadFailed') }}</p>
        <button type="button" class="btn btn-primary mt-4" @click="discover">{{
          t('common.retry')
        }}</button>
      </div>
      <EmptyState
        v-else-if="!panels.length"
        :title="t('plugins.notFound')"
        :description="t('plugins.notFoundDescription')"
      />
      <template v-else>
        <nav class="flex flex-wrap gap-2" :aria-label="t(page?.titleKey ?? 'plugins.pageTitle')">
          <RouterLink
            v-for="panel in panels"
            :key="panel.id"
            :to="{ path: route.path, query: { tab: panel.id } }"
            class="btn"
            :class="active === panel.id ? 'btn-primary' : 'btn-secondary'"
            :aria-current="active === panel.id ? 'page' : undefined"
            >{{ t(panel.titleKey) }}</RouterLink
          >
        </nav>
        <div :key="`${route.path}:${auth.user?.id}:${auth.user?.role}`">
          <section
            v-for="panel in panels"
            v-show="active === panel.id"
            :key="panel.id"
            :aria-label="t(panel.titleKey)"
          >
            <PluginPageContent
              :plugin-id="panel.pluginId"
              :subpath="panel.subpath"
              audience="user"
            />
          </section>
        </div>
      </template>
    </section>
  </AppLayout>
</template>
