<template>
  <AppLayout>
    <template #page-heading-actions>
      <button type="button" class="btn btn-secondary" @click="exportCatalog">
        <Icon name="download" size="sm" aria-hidden="true" />
        {{ t('permissions.export') }}
      </button>
    </template>

    <div class="space-y-4">
      <section class="card p-4 sm:p-5" aria-labelledby="permission-roles">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h2 id="permission-roles" class="font-semibold text-gray-900 dark:text-gray-100">
            {{ t('permissions.roles') }}
          </h2>
          <span class="text-sm text-gray-600 dark:text-gray-300">{{
            t('permissions.readOnly')
          }}</span>
        </div>
        <ul class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
          <li
            v-for="role in roles"
            :key="role.id"
            class="min-w-0 space-y-2 rounded-control border border-gray-200 p-4 dark:border-dark-700"
          >
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{
                role.label
              }}</h3>
              <span class="text-sm text-gray-600 dark:text-gray-300">{{ role.scope }}</span>
            </div>
            <p class="text-sm text-gray-600 dark:text-gray-300">{{ role.rule }}</p>
          </li>
        </ul>
      </section>

      <section class="card" aria-labelledby="permission-resources">
        <div
          class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 p-4 dark:border-dark-700 sm:p-5"
        >
          <h2 id="permission-resources" class="font-semibold text-gray-900 dark:text-gray-100">
            {{ t('permissions.resources') }}
          </h2>
          <div class="input-icon-wrap min-w-0 w-full sm:w-72">
            <Icon name="search" size="sm" class="input-icon text-gray-500" aria-hidden="true" />
            <input
              v-model.trim="query"
              type="search"
              class="input input-has-icon"
              :aria-label="t('permissions.search')"
              :placeholder="t('permissions.search')"
            />
          </div>
        </div>
        <div
          v-if="visibleResources.length === 0"
          role="status"
          class="p-5 text-sm text-gray-600 dark:text-gray-300"
        >
          {{ t('permissions.noResults') }}
        </div>
        <ul v-else class="divide-y divide-gray-200 dark:divide-dark-700">
          <li
            v-for="resource in visibleResources"
            :key="resource.id"
            class="grid grid-cols-1 gap-2 p-4 sm:grid-cols-[minmax(0,12rem)_minmax(0,1fr)] sm:gap-4 sm:p-5"
          >
            <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{
              resource.label
            }}</h3>
            <p class="min-w-0 text-sm text-gray-600 dark:text-gray-300">{{ resource.rule }}</p>
          </li>
        </ul>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { downloadJsonBlob } from '@/utils/download'

// PermissionsView.vue presents the role baselines and resource boundaries enforced
// by the application. It answers what classes of credentials can do before an
// administrator investigates an account's memberships or permissions in MCP.
// @project-doc docs/domains/identity_and_tenancy.md#permissions_reference

// These stable identifiers name the rules included in full catalog exports.
const roleIDs = ['user', 'admin', 'team_owner', 'team_member'] as const
const resourceIDs = [
  'api_keys',
  'routing',
  'usage',
  'billing',
  'support',
  'teams',
  'admin',
  'mcp',
] as const

// The filter searches translated resource text and the stable export identifiers.
const { t } = useI18n()
const query = ref('')
const roles = computed(() =>
  roleIDs.map((id) => ({
    id,
    label: t(`permissions.role.${id}.label`),
    scope: t(`permissions.role.${id}.scope`),
    rule: t(`permissions.role.${id}.rule`),
  })),
)
const resources = computed(() =>
  resourceIDs.map((id) => ({
    id,
    label: t(`permissions.resource.${id}.label`),
    rule: t(`permissions.resource.${id}.rule`),
  })),
)
const visibleResources = computed(() => {
  const needle = query.value.toLocaleLowerCase()
  return resources.value.filter(({ id, label, rule }) =>
    `${id} ${label} ${rule}`.toLocaleLowerCase().includes(needle),
  )
})

// exportCatalog downloads all rules in the active UI language. The search
// filter applies to the page only, so a saved copy remains complete even when
// an administrator exports it while investigating a single resource.
function exportCatalog() {
  downloadJsonBlob(
    { roles: roles.value, resources: resources.value },
    'tokenrouter-permissions.json',
  )
}
</script>
