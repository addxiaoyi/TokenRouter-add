import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import SocialOAuthSettings from '../SocialOAuthSettings.vue'
import { createSettingsForm } from '../settingsState'
import { buildIdentitySettingsPayload } from '../identitySettingsPayload'

// SocialOAuthSettings.spec.ts exercises the administrator's callback display and
// write-only credentials through the shared settings draft. The tests use the
// production serializer to ensure blank secret inputs retain saved credentials
// while provider switches and explicit callbacks survive submission.

// Clipboard feedback is isolated from the browser permission API; the captured
// argument is the exact URL an administrator registers with the provider.
const { copyToClipboard } = vi.hoisted(() => ({ copyToClipboard: vi.fn() }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard }) }))

describe('SocialOAuthSettings', () => {
  // Each provider displays a callback on the current API origin and preserves
  // saved secrets. A newly entered secret is submitted only for that provider.
  it.each(['telegram', 'discord'] as const)(
    'configures %s without returning stored secrets',
    async (provider) => {
      const form = createSettingsForm()
      form[`${provider}_oauth_client_secret_configured`] = true
      const wrapper = mount(SocialOAuthSettings, {
        props: { provider, form },
        global: {
          plugins: [
            createI18n({
              legacy: false,
              locale: 'en',
              messages: {},
              missingWarn: false,
              fallbackWarn: false,
            }),
          ],
        },
      })
      expect(wrapper.get('code').text()).toBe(
        `${window.location.origin}/api/v1/auth/oauth/${provider}/callback`,
      )
      await wrapper.get(`#${provider}-client-id`).setValue('client-id')
      form[`${provider}_oauth_enabled`] = true
      let payload = buildIdentitySettingsPayload(form, 'open')
      expect(payload[`${provider}_oauth_enabled`]).toBe(true)
      expect(payload[`${provider}_oauth_client_id`]).toBe('client-id')
      expect(payload[`${provider}_oauth_client_secret`]).toBeUndefined()
      await wrapper.get(`#${provider}-client-secret`).setValue('new-secret')
      await wrapper.get(`#${provider}-callback`).setValue('https://login.example/custom/callback')
      payload = buildIdentitySettingsPayload(form, 'open')
      expect(payload[`${provider}_oauth_client_secret`]).toBe('new-secret')
      expect(payload[`${provider}_oauth_redirect_url`]).toBe(
        'https://login.example/custom/callback',
      )
      await wrapper.get('button.btn').trigger('click')
      expect(copyToClipboard).toHaveBeenCalledWith(
        'https://login.example/custom/callback',
        expect.any(String),
      )
      wrapper.unmount()
    },
  )
})
