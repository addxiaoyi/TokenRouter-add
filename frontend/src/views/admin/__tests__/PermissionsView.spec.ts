import { mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import permissions from '@/i18n/locales/en/permissions'
import { downloadJsonBlob } from '@/utils/download'
import PermissionsView from '../PermissionsView.vue'

// PermissionsView.spec.ts checks the visible reference and its complete export.
// It uses the shipped labels to exercise a real search while intercepting the
// browser download at the shared file helper.

vi.mock('@/utils/download', () => ({ downloadJsonBlob: vi.fn() }))
vi.mock('vue-i18n', async (original) => ({
  ...(await original<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string) =>
      key
        .split('.')
        .slice(1)
        .reduce<unknown>((value, part) => {
          if (value && typeof value === 'object' && Object.hasOwn(value, part)) {
            return (value as Record<string, unknown>)[part]
          }
          return key
        }, permissions),
  }),
}))

// The support filter hides unrelated rules but leaves every resource in the
// exported reference. A search with no matches also announces the empty state.
it('filters resource rules without narrowing the exported reference', async () => {
  const page = mount(PermissionsView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot name="page-heading-actions" /><slot /></main>' },
        Icon: true,
      },
    },
  })

  await page.get('input[type="search"]').setValue('tickets')
  const resources = page.get('section[aria-labelledby="permission-resources"]')
  expect(resources.findAll('li').map((row) => row.get('h3').text())).toEqual(['Support tickets'])

  await page.get('button').trigger('click')
  expect(downloadJsonBlob).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({
      roles: expect.arrayContaining([expect.objectContaining({ id: 'team_owner' })]),
      resources: expect.arrayContaining([expect.objectContaining({ id: 'api_keys' })]),
    }),
    'tokenrouter-permissions.json',
  )

  await page.get('input[type="search"]').setValue('missing resource')
  expect(resources.get('[role="status"]').text()).toBe(permissions.noResults)
})
