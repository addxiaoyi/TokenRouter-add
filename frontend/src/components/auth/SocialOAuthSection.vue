<template>
  <button type="button" :disabled="disabled" class="btn btn-secondary w-full" @click="startLogin">
    <AuthProviderIcon :provider="provider" class="mr-2 h-5 w-5" />
    {{ t('auth.social.signIn', { provider: provider === 'telegram' ? 'Telegram' : 'Discord' }) }}
  </button>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { OAuthLoginStart } from '@/api/auth'
import { resolveAffiliateCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'
import AuthProviderIcon from './AuthProviderIcon.vue'

// SocialOAuthSection.vue emits Telegram and Discord authorization starts through
// the parent login gate. It retains referral and promotion context while the
// parent controls consent, CAPTCHA, loading feedback, and navigation to the
// server-generated authorization URL.

// Provider selection and referral drafts belong to the enclosing login form;
// this component emits a request only and never installs authentication tokens.
const props = defineProps<{
  provider: 'telegram' | 'discord' // fixed social provider selection
  disabled?: boolean // parent authentication action state
  affCode?: string // optional referral attribution code
  promoCode?: string // optional signup promotion code
}>()
const emit = defineEmits<{
  start: [request: OAuthLoginStart] // CAPTCHA-gated authorization start request
}>()
const route = useRoute() // current local return destination
const { t } = useI18n() // localized sign-in button labels

// startLogin emits the selected social provider through the shared admission gate.
// Referral context is saved before navigation. The backend validates the return
// path and owns all authorization credentials and browser ceremony cookies.
function startLogin(): void {
  // Keep attribution before provider navigation so account creation can recover
  // the referral after the browser returns through the separate callback route.
  storeOAuthAffiliateCode(
    resolveAffiliateCode(props.affCode, route.query.aff, route.query.aff_code),
  )
  const params: Record<string, string> = {
    redirect: typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard',
  }
  if (props.promoCode?.trim()) {
    params.promo_code = props.promoCode.trim()
  }
  emit('start', { provider: props.provider, params })
}
</script>
