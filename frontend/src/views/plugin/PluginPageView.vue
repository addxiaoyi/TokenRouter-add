<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import PluginPageContent from './PluginPageContent.vue'

// PluginPageView.vue maps authenticated plugin routes to the shared content host.
// The content host owns discovery, bundle loading, and failure recovery so single
// plugin routes and combined account pages use the same scoped capability boundary.

// Route metadata selects audience; route parameters identify a static contribution
// path without allowing a user route to request administrator-only page capabilities.
const route = useRoute()
const pluginId = computed(() =>
  typeof route.params.pluginId === 'string' ? route.params.pluginId : '',
)
const audience = computed(() => (route.meta.requiresAdmin ? 'admin' : 'user'))
const subpath = computed(() => {
  const parts = route.params.subpath
  return Array.isArray(parts) ? parts.join('/') : parts || ''
})
</script>

<template>
  <AppLayout :hide-page-heading="true">
    <PluginPageContent :plugin-id="pluginId" :subpath="subpath" :audience="audience" />
  </AppLayout>
</template>
