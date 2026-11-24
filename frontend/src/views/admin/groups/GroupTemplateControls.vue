<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { GroupFormDraft } from './groupFormDraft'
import {
  groupTemplatePolicy,
  MAX_GROUP_TEMPLATE_NAME,
  MAX_GROUP_TEMPLATES,
  readGroupTemplates,
  writeGroupTemplates,
  type GroupTemplate,
  type GroupTemplatePolicy,
} from './groupTemplates'

// GroupTemplateControls.vue applies browser templates to a new group draft.
// The controls read the signed-in administrator's library and use the current
// form values only when saving. Applying a template emits an allowlisted patch
// through the editor, where the regular group creation validation still runs.

// The parent owns the editable group draft and submits it through its regular form.
const props = defineProps<{
  draft: GroupFormDraft // Current group draft.
  disabled: boolean // Prevents edits while submitting.
}>()
const emit = defineEmits<{
  apply: [policy: GroupTemplatePolicy] // Allowlisted fields for the draft.
}>()
const { t } = useI18n()
const app = useAppStore()
const auth = useAuthStore()

// Template state follows the administrator account that owns this browser library.
const templates = ref<GroupTemplate[]>([])
const selectedName = ref('')
const newName = ref('')
const unavailable = ref(false)
const selected = computed(() => templates.value.find((entry) => entry.name === selectedName.value))
const options = computed(() =>
  templates.value.map((entry) => ({ label: entry.name, value: entry.name })),
)

watch(
  () => auth.user?.id,
  (userId) => {
    selectedName.value = ''
    newName.value = ''
    templates.value = []
    unavailable.value = !userId
    if (!userId) return
    try {
      templates.value = readGroupTemplates(userId)
    } catch {
      // A blocked or corrupt browser library leaves group creation available and
      // shows a status message so administrators know why templates are disabled.
      unavailable.value = true
    }
  },
  { immediate: true },
)

// saveTemplate validates the current draft's reusable values and stores a new
// name for the active account. Existing names and full libraries produce
// explicit feedback. The current form values are kept if storage fails.
function saveTemplate() {
  const userId = auth.user?.id
  const name = newName.value.trim()
  if (!userId || !name || props.disabled) return
  let policy: GroupTemplatePolicy
  try {
    policy = groupTemplatePolicy(props.draft)
  } catch {
    // The template cannot capture a partially entered multiplier. Keep the
    // draft editable and ask the administrator to complete that field first.
    app.showError(t('groupTemplates.invalidPolicy'))
    return
  }
  try {
    // Reread the current library before writing so templates saved earlier
    // in another tab are retained and participate in duplicate checks.
    const current = readGroupTemplates(userId)
    if (current.length >= MAX_GROUP_TEMPLATES) {
      app.showError(t('groupTemplates.libraryFull', { count: MAX_GROUP_TEMPLATES }))
      return
    }
    if (current.some((entry) => entry.name.toLowerCase() === name.toLowerCase())) {
      app.showError(t('groupTemplates.duplicateName'))
      return
    }
    const next = [...current, { name, policy }]
    writeGroupTemplates(userId, next)
    templates.value = next
    selectedName.value = name
    newName.value = ''
    app.showSuccess(t('groupTemplates.saved'))
  } catch {
    app.showError(t('groupTemplates.storageFailed'))
  }
}

// applyTemplate fills the current creation draft after an explicit click.
// Only the selected policy fields are patched. The existing form remains
// editable and the administrator creates the group through its normal submit button.
function applyTemplate() {
  if (!selected.value || props.disabled) return
  emit('apply', selected.value.policy)
  app.showSuccess(t('groupTemplates.applied'))
}

// removeTemplate removes the selected entry from this account's local library.
// Browser failures retain the selection and show feedback. The active group
// draft stays intact even if it was filled using the removed entry.
function removeTemplate() {
  const userId = auth.user?.id
  if (!userId || !selected.value || props.disabled) return
  try {
    // Remove from the latest list so entries written by another tab are preserved.
    const next = readGroupTemplates(userId).filter((entry) => entry.name !== selectedName.value)
    writeGroupTemplates(userId, next)
    templates.value = next
    selectedName.value = ''
    app.showSuccess(t('groupTemplates.removed'))
  } catch {
    app.showError(t('groupTemplates.storageFailed'))
  }
}
</script>

<template>
  <details class="rounded-surface border border-gray-200 p-4 dark:border-dark-700">
    <summary class="cursor-pointer text-sm font-medium">{{ t('groupTemplates.title') }}</summary>
    <div class="space-y-3 pt-3">
      <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('groupTemplates.scope') }}</p>
      <p v-if="unavailable" role="status" class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('groupTemplates.unavailable') }}
      </p>
      <template v-else>
        <div v-if="templates.length" class="flex flex-wrap items-center gap-2">
          <Select
            v-model="selectedName"
            :options="options"
            :aria-label="t('groupTemplates.choose')"
            :placeholder="t('groupTemplates.choose')"
            :disabled="disabled"
            class="min-w-36 flex-1"
          />
          <button
            type="button"
            :disabled="!selected || disabled"
            class="btn btn-secondary btn-sm"
            @click="applyTemplate"
          >
            {{ t('groupTemplates.apply') }}
          </button>
          <button
            type="button"
            :disabled="!selected || disabled"
            class="btn btn-secondary btn-sm"
            @click="removeTemplate"
          >
            {{ t('groupTemplates.remove') }}
          </button>
        </div>
        <div class="flex flex-wrap items-end gap-2">
          <label class="min-w-36 flex-1">
            <span class="input-label">{{ t('groupTemplates.name') }}</span>
            <input
              v-model="newName"
              type="text"
              :maxlength="MAX_GROUP_TEMPLATE_NAME"
              :disabled="disabled"
              autocomplete="off"
              class="input"
              @keydown.enter.prevent="saveTemplate"
            />
          </label>
          <button
            type="button"
            :disabled="!newName.trim() || disabled"
            class="btn btn-secondary btn-sm"
            @click="saveTemplate"
          >
            {{ t('groupTemplates.save') }}
          </button>
        </div>
      </template>
    </div>
  </details>
</template>
