<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminGroup } from '@/api/groups.types'

// GroupHealthDetails.vue separates a group's saved activation setting from its
// current provider-count snapshot and explains the configured fallback targets.
// Enabled groups can have no linked providers, linked providers without a
// currently schedulable candidate, or candidates whose model and protocol still
// require request-specific checks. These cases remain distinct so a green
// activation badge cannot imply a successful live probe or guaranteed recovery.
//
// The expanded explanation lists all three direct fallback policies without
// changing their saved configuration. Names come from the parent's loaded group
// choices; an absent target remains unverified rather than being declared
// deleted or healthy. Execution still belongs to the gateway, which checks the
// request's key permissions, model, protocol, and billing constraints. A group
// snapshot alone cannot establish that a particular request can use a fallback.

// GroupHealthRecord contains the saved activation, direct fallback policies,
// and provider counts needed to explain one administrator group row.
type GroupHealthRecord = Pick<
  AdminGroup,
  | 'id'
  | 'status'
  | 'provider_count'
  | 'active_provider_count'
  | 'rate_limited_provider_count'
  | 'fallback_group_id'
  | 'fallback_group_id_on_invalid_request'
  | 'unavailable_fallback_group_id'
>

// props borrows a group row and optional target-name snapshots; no operations in
// this component mutate either the row or its parent's fallback configuration.
const props = withDefaults(
  defineProps<{
    group: GroupHealthRecord // current administrative group snapshot
    fallbackGroups?: Pick<AdminGroup, 'id' | 'name' | 'status'>[] // loaded fallback target snapshots
  }>(),
  { fallbackGroups: () => [] },
)

// Translation supplies activation and policy explanations using the current
// administrator language, including labels for unverified fallback target records.
const { t } = useI18n()

// fallbackPolicies pairs each saved direct target with its independently
// triggered policy. Missing target snapshots retain the saved identifier for inspection.
const fallbackPolicies = computed(() =>
  [
    { key: 'client', id: props.group.fallback_group_id },
    { key: 'invalid', id: props.group.fallback_group_id_on_invalid_request },
    { key: 'unavailable', id: props.group.unavailable_fallback_group_id },
  ].map((policy) => ({
    ...policy,
    target: props.fallbackGroups.find((group) => group.id === policy.id),
  })),
)
</script>

<template>
  <div class="space-y-1 text-xs">
    <span :class="['badge', group.status === 'active' ? 'badge-success' : 'badge-gray']">
      {{ t(`admin.groups.health.activation.${group.status}`) }}
    </span>
    <div
      v-if="group.status !== 'active'"
      class="text-gray-500 dark:text-gray-400"
      data-health-state="disabled"
    >
      {{ t('admin.groups.health.disabledHint') }}
    </div>
    <div
      v-else-if="!group.provider_count"
      class="text-amber-600 dark:text-amber-400"
      data-health-state="empty"
    >
      {{ t('admin.groups.health.noProviders') }}
    </div>
    <div
      v-else-if="!group.active_provider_count"
      class="text-amber-600 dark:text-amber-400"
      data-health-state="unavailable"
    >
      {{ t('admin.groups.health.noCandidates') }}
    </div>
    <div v-else class="text-gray-600 dark:text-gray-300" data-health-state="candidates">
      {{ t('admin.groups.health.hasCandidates', { count: group.active_provider_count }) }}
    </div>
    <details class="max-w-72 whitespace-normal break-words">
      <summary class="cursor-pointer text-gray-500 dark:text-gray-400">{{
        t('admin.groups.health.details')
      }}</summary>
      <div class="mt-2 space-y-2 text-gray-600 dark:text-gray-300">
        <p>{{
          t('admin.groups.health.counts', {
            active: group.active_provider_count || 0,
            limited: group.rate_limited_provider_count || 0,
            total: group.provider_count || 0,
          })
        }}</p>
        <p>{{ t('admin.groups.health.snapshotHint') }}</p>
        <ul class="space-y-1">
          <li v-for="policy in fallbackPolicies" :key="policy.key">
            {{ t(`admin.groups.health.fallback.${policy.key}`) }}:
            <template v-if="policy.id">
              #{{ policy.id }} {{ policy.target?.name }}
              <span v-if="policy.id === group.id" class="text-amber-600 dark:text-amber-400">{{
                t('admin.groups.health.selfFallback')
              }}</span>
              <span v-else-if="!policy.target">{{
                t('admin.groups.health.targetUnverified')
              }}</span>
              <span
                v-else-if="policy.target.status !== 'active'"
                class="text-amber-600 dark:text-amber-400"
                >{{ t('admin.groups.health.targetDisabled') }}</span
              >
            </template>
            <span v-else>{{ t('admin.groups.health.notConfigured') }}</span>
          </li>
        </ul>
        <p>{{ t('admin.groups.health.fallbackHint') }}</p>
      </div>
    </details>
  </div>
</template>
