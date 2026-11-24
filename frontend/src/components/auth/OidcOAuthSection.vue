<template>
  <div class="space-y-4">
    <button type="button" :disabled="disabled" class="btn btn-secondary w-full" @click="startLogin">
      <AuthProviderIcon provider="oidc" class="h-5 w-5" />
      <span class="min-w-0 break-words [overflow-wrap:anywhere]">
        {{ t('auth.oidc.signIn', { providerName: normalizedProviderName }) }}
      </span>
    </button>

    <div v-if="showDivider" class="flex items-center gap-3">
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
      <span class="text-xs text-gray-500 dark:text-dark-400">
        {{ t('auth.oauthOrContinue') }}
      </span>
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { OAuthLoginStart } from '@/api/auth'
import { resolveAffiliateCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'
import AuthProviderIcon from './AuthProviderIcon.vue'

// OidcOAuthSection.vue presents the OIDC sign-in entry and emits its start request
// to the parent authentication gate. The configured display name supplies the
// label, with OIDC used for missing or blank names. The parent
// performs captcha checks and navigation after receiving the event.

// props supplies parent-owned configuration and draft input. emit reports
// authentication actions back to that owner, which decides whether to submit or
// navigate.
const props = withDefaults(
  defineProps<{
    disabled?: boolean // parent action disable flag

    affCode?: string // parent referral code

    // providerName defaults to OIDC when omitted. The displayed label also falls
    // back after trimming without changing the fixed OAuth provider identifier.
    providerName?: string // configured provider label

    // showDivider defaults to true when omitted. Parents can hide the separator
    // when composing several provider sections in the same authentication form.
    showDivider?: boolean // provider separator visibility
  }>(),
  {
    providerName: 'OIDC',
    showDivider: true,
  },
) // parent component inputs
const emit = defineEmits<{
  start: [request: OAuthLoginStart] // parent authentication start request
}>() // parent component event dispatcher

// route and router connect callback or sign-in handlers to query values and
// navigation. Authentication stores and translators supply the session actions and
// notifications used by those handlers.
const route = useRoute() // current callback query context
const { t } = useI18n() // translated authentication message resolver

// normalizedProviderName supplies the configured sign-in label after trimming.
// Missing or blank names display OIDC; the label remains separate from the fixed
// provider identifier sent in authorization requests.
const normalizedProviderName = computed(() => {
  const name = props.providerName?.trim()
  return name || 'OIDC'
}) // trimmed OIDC display label

// startLogin emits the Oidc request with the current redirect or /dashboard
// fallback. It saves resolved referral attribution first, leaving consent checks,
// action captcha, and provider navigation to the parent.
function startLogin(): void {
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  // Save referral attribution before leaving the current sign-in entry, allowing
  // the callback to include it if the provider requires account creation.
  storeOAuthAffiliateCode(
    resolveAffiliateCode(props.affCode, route.query.aff, route.query.aff_code),
  )
  emit('start', { provider: 'oidc', params: { redirect: redirectTo } })
}
</script>
