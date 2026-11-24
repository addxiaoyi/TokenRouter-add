import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import SocialOAuthSection from '../SocialOAuthSection.vue'

// SocialOAuthSection.spec.ts checks provider selection and parent-controlled
// admission at the sign-in button boundary. Authorization remains an emitted
// request so the enclosing consent and CAPTCHA flow can approve navigation,
// and disabled controls cannot initiate another concurrent request.

vi.mock('vue-router', () => ({ useRoute: () => ({ query: { redirect: '/profile' } }) }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (_key: string, params: { provider: string }) => `Continue with ${params.provider}`,
  }),
}))

describe('SocialOAuthSection', () => {
  // Both fixed social providers carry the return path and promotion through the
  // parent gate. Disabling the component suppresses further authorization requests.
  it.each(['telegram', 'discord'] as const)(
    'emits %s through the parent admission gate',
    async (provider) => {
      const wrapper = mount(SocialOAuthSection, {
        props: { provider, promoCode: ' promo ' },
        global: { stubs: { AuthProviderIcon: true } },
      })
      await wrapper.get('button').trigger('click')
      expect(wrapper.emitted('start')).toEqual([
        [{ provider, params: { redirect: '/profile', promo_code: 'promo' } }],
      ])
      await wrapper.setProps({ disabled: true })
      await wrapper.get('button').trigger('click')
      expect(wrapper.emitted('start')).toHaveLength(1)
      wrapper.unmount()
    },
  )
})
