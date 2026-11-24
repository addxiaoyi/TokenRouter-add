<template>
  <SettingsCard :title="title" :description="t(`admin.settings.social.${provider}Hint`)">
    <SettingsSection>
      <SettingToggleRow
        :id="`${provider}-oauth-enabled`"
        v-model="form[`${provider}_oauth_enabled`]"
        :label="t('admin.settings.social.enable', { provider: title })"
      />
      <div class="grid gap-4 md:grid-cols-2">
        <div>
          <label :for="`${provider}-client-id`" class="input-label">Client ID</label>
          <input
            :id="`${provider}-client-id`"
            v-model.trim="form[`${provider}_oauth_client_id`]"
            class="input font-mono text-sm"
            autocomplete="off"
          />
        </div>
        <div>
          <label :for="`${provider}-client-secret`" class="input-label">Client Secret</label>
          <input
            :id="`${provider}-client-secret`"
            v-model="form[`${provider}_oauth_client_secret`]"
            type="password"
            class="input font-mono text-sm"
            autocomplete="new-password"
            :placeholder="
              t(
                form[`${provider}_oauth_client_secret_configured`]
                  ? 'admin.settings.social.secretSaved'
                  : 'admin.settings.social.secretNeeded',
              )
            "
          />
          <p class="input-hint">{{ t('admin.settings.social.secretHint') }}</p>
        </div>
      </div>
      <div>
        <label :for="`${provider}-callback`" class="input-label">{{
          t('admin.settings.social.callback')
        }}</label>
        <div class="flex gap-2">
          <input
            :id="`${provider}-callback`"
            v-model.trim="form[`${provider}_oauth_redirect_url`]"
            type="url"
            class="input font-mono text-sm"
            :placeholder="callbackSuggestion"
          />
          <button type="button" class="btn btn-secondary btn-sm shrink-0" @click="copyCallback">{{
            t('admin.settings.social.copyCallback')
          }}</button>
        </div>
        <p class="input-hint">{{ t('admin.settings.social.callbackHint') }}</p>
        <code class="block select-all break-all text-sm">{{ effectiveCallback }}</code>
      </div>
      <a
        :href="
          provider === 'telegram'
            ? 'https://core.telegram.org/bots/telegram-login'
            : 'https://discord.com/developers/applications'
        "
        target="_blank"
        rel="noopener noreferrer"
        class="text-link text-sm"
        >{{ t('admin.settings.social.setupLink') }}</a
      >
    </SettingsSection>
  </SettingsCard>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SettingsCard from '@/components/common/settings/SettingsCard.vue'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import { useClipboard } from '@/composables/useClipboard'
import { buildApiUrl } from '@/api/url'
import type { IdentitySettingsForm } from './useIdentitySettings'

// SocialOAuthSettings.vue edits Telegram or Discord credentials in the shared
// administrator draft. The displayed callback follows the browser's configured
// API origin until an administrator supplies an explicit registered address;
// copying it does not mutate the draft or persist settings.

// The enclosing settings page owns persistence and clears secret inputs after
// successful saves. Each instance edits only fields for its fixed provider.
const props = defineProps<{
  provider: 'telegram' | 'discord' // fixed authorization provider selection
  form: IdentitySettingsForm // shared administrator settings draft
}>()
// form retains the page-owned mutable draft; the enclosing save action persists
// edits together with the other identity settings and resets credentials afterward.
const { form } = props
const { t } = useI18n() // localized setup guidance and hints
const { copyToClipboard } = useClipboard() // shared clipboard feedback owner
// Provider labels and callback display derive from the selected card and API
// origin. An explicit callback draft takes precedence until the page saves it.
const title = computed(() => (props.provider === 'telegram' ? 'Telegram' : 'Discord'))
const callbackSuggestion = computed(() =>
  new URL(buildApiUrl(`/auth/oauth/${props.provider}/callback`), window.location.origin).toString(),
)
const effectiveCallback = computed(
  () => props.form[`${props.provider}_oauth_redirect_url`] || callbackSuggestion.value,
)

// copyCallback copies the effective registration address with shared clipboard feedback.
// Empty callback drafts use the current API origin. Saving credentials remains
// the enclosing settings page's explicit action after the platform setup is complete.
async function copyCallback(): Promise<void> {
  await copyToClipboard(effectiveCallback.value, t('admin.settings.social.callbackCopied'))
}
</script>
