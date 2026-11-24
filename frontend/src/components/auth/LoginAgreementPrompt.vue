<template>
  <div v-if="mode === 'checkbox' && documents.length > 0" class="px-0.5">
    <div class="flex items-start gap-2">
      <!--
        Anchor the missing-consent reminder to the checkbox so its focus target
        stays distinct from the wrapping legal-document links beside it.
      -->
      <HelpTooltip
        trigger="manual"
        :open="hintVisible && !accepted"
        tooltip-id="login-agreement-hint"
        :content="t('auth.agreementRequired')"
        :closable="false"
        width-class="w-72"
        class="mt-0.5 shrink-0"
        @update:open="emit('update:hintVisible', $event)"
      >
        <template #trigger>
          <input
            id="login-agreement-consent"
            ref="checkboxRef"
            type="checkbox"
            :checked="accepted"
            :aria-describedby="hintVisible && !accepted ? 'login-agreement-hint' : undefined"
            :aria-invalid="hintVisible && !accepted ? true : undefined"
            class="h-4 w-4 flex-shrink-0 rounded-compact border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-900"
            @change="handleCheckboxChange"
          />
        </template>
      </HelpTooltip>
      <div class="min-w-0 flex-1">
        <p class="text-[13px] leading-5 text-gray-600 dark:text-dark-300">
          <label
            for="login-agreement-consent"
            class="cursor-pointer text-gray-700 dark:text-dark-200"
          >
            {{ t('legal.acceptedPrefix') }}
          </label>
          <template v-for="(doc, index) in documents" :key="doc.id || doc.title">
            <RouterLink
              :to="documentRoute(doc)"
              target="_blank"
              rel="noopener noreferrer"
              class="font-medium text-link"
            >
              {{ doc.title }}
            </RouterLink>
            <span v-if="index < documents.length - 1">、</span>
          </template>
        </p>
      </div>
    </div>
  </div>

  <div
    v-else-if="!accepted && documents.length > 0"
    class="rounded-surface border border-primary-100 bg-primary-50/70 p-3 text-sm text-primary-900 dark:border-primary-500/20 dark:bg-primary-500/10 dark:text-primary-100"
  >
    <div class="flex items-start gap-3">
      <Icon
        name="shield"
        size="sm"
        class="mt-0.5 flex-shrink-0 text-primary-600 dark:text-primary-300"
      />
      <div class="min-w-0 flex-1">
        <p class="font-medium">{{ t('legal.consentRequired') }}</p>
        <p class="mt-1 text-primary-700 dark:text-primary-200/80">
          {{ t('legal.disabledUntilAccepted') }}
        </p>
      </div>
      <button type="button" class="btn btn-primary btn-sm shrink-0" @click="emit('open')">
        {{ t('legal.viewTerms') }}
      </button>
    </div>
  </div>

  <BaseDialog
    :show="dialogVisible"
    :title="t('legal.updateNotice')"
    :z-index="Z_INDEX.ANNOUNCEMENT_TOP"
    :close-on-click-outside="false"
    trap-focus
    @close="emit('reject')"
  >
    <p class="text-sm leading-6 text-gray-600 dark:text-dark-300">
      {{ t('legal.changedNotice', { date: updatedAt || t('legal.recently') }) }}
    </p>

    <ul
      :aria-label="t('legal.documents')"
      class="mt-4 divide-y divide-gray-200 dark:divide-dark-700"
    >
      <li v-for="doc in documents" :key="doc.id || doc.title">
        <RouterLink
          :to="documentRoute(doc)"
          target="_blank"
          rel="noopener noreferrer"
          class="text-link-area flex min-h-12 items-center gap-4 rounded-control px-2 py-3 hover:bg-gray-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-600 dark:hover:bg-dark-800 dark:focus-visible:outline-primary-500"
        >
          <span class="text-link min-w-0 flex-1 break-words text-sm font-medium">
            {{ doc.title }}
          </span>
          <Icon name="externalLink" size="sm" class="shrink-0 text-gray-400 dark:text-dark-400" />
        </RouterLink>
      </li>
    </ul>

    <template #footer>
      <div class="flex w-full flex-wrap justify-end gap-3">
        <button type="button" class="btn btn-secondary min-w-20" @click="emit('reject')">
          {{ t('legal.reject') }}
        </button>
        <button
          type="button"
          class="btn btn-primary min-w-40 flex-1 sm:flex-none"
          @click="emit('accept')"
        >
          {{ t('legal.accept') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import BaseDialog from '@/components/common/BaseDialog.vue'
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import type { LoginAgreementDocument } from '@/api/settings.types'
import { Z_INDEX } from '@/constants/overlay'

// LoginAgreementPrompt.vue renders checkbox or modal consent controls for
// configured legal documents. The parent owns acceptance and persistence, while
// this component filters blank titles, emits user decisions, and focuses the
// checkbox when an action needs consent. The shared dialog shell owns focus
// containment and scrolling; dismissing that shell reports rejection to the parent.

// props supplies parent-owned configuration and draft input. emit reports
// authentication actions back to that owner, which decides whether to submit or
// navigate.
const props = withDefaults(
  defineProps<{
    // accepted is owned by the parent consent flow. This component emits decisions
    // and waits for updated props rather than persisting acceptance itself.
    accepted: boolean // parent consent decision

    // documents supplies the legal entries owned by the parent. Entries with blank
    // titles are filtered before rendering links or deciding whether the dialog
    // can appear.
    documents: LoginAgreementDocument[] // configured legal documents

    // mode selects checkbox presentation only for the literal checkbox value.
    // Other values normalize to modal presentation for the current document list.
    mode: 'modal' | 'checkbox' | string // consent presentation mode

    updatedAt?: string // displayed agreement update date

    visible: boolean // parent dialog visibility

    // hintVisible asks the component to focus and reveal the checkbox after
    // rendering. The reminder is displayed only while consent remains unaccepted.
    hintVisible?: boolean // checkbox reminder visibility
  }>(),
  {
    updatedAt: '',
  },
) // parent component inputs

const emit = defineEmits<{
  accept: [] // user accepted current agreement

  reject: [] // user rejected current agreement

  open: [] // user requested agreement dialog

  'update:hintVisible': [value: boolean] // updated checkbox reminder visibility
}>() // parent component event dispatcher

// t translates consent instructions and document actions for the active locale.
// checkboxRef provides the separate focus target used when the reminder becomes
// visible.
const { t } = useI18n() // translated authentication message resolver
// checkboxRef is the focus target when a blocked authentication action reveals the
// consent hint. Its DOM handle can be absent until checkbox mode renders.
const checkboxRef = ref<HTMLInputElement | null>(null) // consent reminder focus target
watch(
  () => props.hintVisible,
  async (visible) => {
    if (!visible) return
    await nextTick()
    // Provider buttons may sit below the consent control. Bring the matching
    // checkbox into view after rendering and focus it for keyboard acceptance.
    checkboxRef.value?.focus({ preventScroll: true })
    checkboxRef.value?.scrollIntoView?.({ block: 'nearest' })
  },
)

// dialogVisible requires both the parent visibility flag and a nonempty filtered
// document list. A dialog without a legal document does not open.
const dialogVisible = computed(() => props.visible && documents.value.length > 0) // document-backed agreement dialog visibility
// documents removes entries with blank titles before consent links render.
// updatedAt and accepted reflect the parent values used by the prompt copy and
// visibility.
const documents = computed(() => props.documents.filter((doc) => doc.title.trim())) // legal documents with nonblank titles
const updatedAt = computed(() => props.updatedAt || '') // displayed agreement update date
const accepted = computed(() => props.accepted) // parent-owned consent state
// mode chooses checkbox presentation only for the corresponding literal value.
// Other configured strings use the modal prompt so the template has two
// presentation branches.
const mode = computed(() => (props.mode === 'checkbox' ? 'checkbox' : 'modal')) // normalized consent presentation mode

// documentRoute builds the legal-document route using its identifier or title
// fallback. Documents without an identifier remain linkable by title, matching the
// filtered document list shown by the consent prompt.
function documentRoute(doc: LoginAgreementDocument) {
  return {
    name: 'LegalDocument',
    params: {
      documentId: doc.id || doc.title,
    },
  }
}

// handleCheckboxChange emits acceptance or rejection from the checkbox DOM state.
// The parent owns persistence and the accepted prop, so this handler reports user
// intent without changing either locally.
function handleCheckboxChange(event: Event): void {
  const checked = (event.target as HTMLInputElement).checked
  if (checked) {
    emit('accept')
  } else {
    emit('reject')
  }
}
</script>
