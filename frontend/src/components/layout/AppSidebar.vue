<template>
  <aside
    class="sidebar"
    :class="[
      sidebarCollapsed ? 'w-[var(--sidebar-w-collapsed)]' : 'w-[var(--sidebar-w)]',
      { '-translate-x-full lg:translate-x-0': !mobileOpen },
    ]"
  >
    <!-- Full-height Codex sidebar top header: brand identity and rail collapse toggle. -->
    <div
      class="flex h-[var(--header-h)] shrink-0 items-center border-b border-black/[0.07] px-2.5 dark:border-white/[0.08]"
      :class="sidebarCollapsed ? 'justify-center' : 'justify-between gap-2'"
    >
      <router-link
        v-if="!sidebarCollapsed"
        to="/home"
        class="group/brand flex min-w-0 items-center gap-2 rounded-control px-1 py-1 transition-colors duration-fast hover:bg-black/[0.04] dark:hover:bg-white/[0.05]"
        :aria-label="siteName"
      >
        <span
          class="flex h-6 w-6 shrink-0 items-center justify-center overflow-hidden rounded-compact bg-gray-100 dark:bg-dark-800"
        >
          <img
            :src="siteLogo || '/logo.svg'"
            :alt="siteName"
            class="h-full w-full object-contain"
          />
        </span>
        <span class="min-w-0 truncate text-sm font-semibold tracking-[-0.015em] text-gray-950 dark:text-white">
          {{ siteName }}
        </span>
        <VersionBadge v-if="siteVersion" :version="siteVersion" class="shrink-0" />
      </router-link>

      <button
        type="button"
        class="hidden h-7 w-7 shrink-0 items-center justify-center rounded-control text-gray-500 transition-colors duration-fast hover:bg-black/[0.05] hover:text-gray-950 lg:inline-flex dark:text-dark-400 dark:hover:bg-white/[0.07] dark:hover:text-white"
        :title="sidebarCollapsed ? t('common.expand') : t('common.collapse')"
        :aria-label="sidebarCollapsed ? t('common.expand') : t('common.collapse')"
        @click="appStore.toggleSidebar()"
      >
        <Icon :name="sidebarCollapsed ? 'chevronRight' : 'chevronLeft'" size="sm" />
      </button>

      <button
        type="button"
        class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-control text-gray-500 transition-colors duration-fast hover:bg-black/[0.05] hover:text-gray-950 lg:hidden dark:text-dark-400 dark:hover:bg-white/[0.07] dark:hover:text-white"
        :aria-label="t('common.close')"
        @click="closeMobile"
      >
        <Icon name="x" size="sm" />
      </button>
    </div>

    <!-- Scrollable Navigation -->
    <nav ref="sidebarNavRef" class="sidebar-nav scrollbar-hide">
      <!-- Admin View: Semantic Codex groups followed by personal account destinations -->
      <template v-if="isAdmin">
        <div
          v-for="(section, sectionIndex) in groupedAdminSections"
          :key="section.key"
          class="sidebar-section"
          :class="{ 'mt-0': sectionIndex === 0 }"
        >
          <div
            class="sidebar-section-title mb-1 px-2.5 text-xs font-medium tracking-[0.04em] text-gray-500/85 dark:text-dark-400/85"
            :class="{ 'sidebar-section-title-collapsed': sidebarCollapsed }"
            :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
          >
            <span
              class="sidebar-section-title-text"
              :class="{ 'sidebar-section-title-text-collapsed': sidebarCollapsed }"
            >
              {{ section.label }}
            </span>
          </div>

          <template v-for="item in section.items" :key="item.path">
            <!-- Collapsible group (has children) -->
            <template v-if="item.children?.length">
              <button
                type="button"
                class="sidebar-link mb-0.5 w-full"
                :class="{
                  'sidebar-link-active': isGroupActive(item) && !isGroupExpanded(item),
                  'sidebar-link-collapsed': sidebarCollapsed,
                }"
                :title="sidebarCollapsed ? item.label : undefined"
                :aria-expanded="!sidebarCollapsed && isGroupExpanded(item)"
                :aria-controls="`sidebar-group-${item.path}`"
                @click="sidebarCollapsed ? undefined : toggleGroup(item)"
              >
                <Icon :name="item.icon ?? 'home'" size="sm" class="h-4 w-4 flex-shrink-0" />
                <span
                  class="sidebar-label sidebar-label-flex flex-1"
                  :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
                  :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
                >
                  <span class="min-w-0 truncate">{{ item.label }}</span>
                  <Icon
                    name="chevronDown"
                    size="xs"
                    :animate-on-hover="false"
                    class="h-3.5 w-3.5 flex-shrink-0 opacity-60 transition-transform duration-normal"
                    :class="isGroupExpanded(item) ? 'rotate-180' : ''"
                  />
                </span>
              </button>
              <!-- Children -->
              <Collapse
                :id="`sidebar-group-${item.path}`"
                :open="!sidebarCollapsed && isGroupExpanded(item)"
                unmount-on-hide
              >
                <div class="mb-1 ml-3.5 border-l border-black/[0.08] pl-2 dark:border-white/[0.08]">
                  <router-link
                    v-for="child in item.children"
                    :key="child.path"
                    :to="child.path"
                    class="sidebar-link mb-0.5 h-7 py-1 text-xs"
                    :class="{ 'sidebar-link-active': route.path === child.path }"
                    @click="handleMenuItemClick(child.path)"
                  >
                    <Icon :name="child.icon ?? 'home'" size="xs" class="h-3.5 w-3.5 flex-shrink-0 opacity-75" />
                    <span class="min-w-0 truncate">{{ child.label }}</span>
                  </router-link>
                </div>
              </Collapse>
            </template>

            <!-- Normal item (no children) -->
            <router-link
              v-else
              :to="item.path"
              class="sidebar-link mb-0.5"
              :class="{
                'sidebar-link-active': isActive(item.path),
                'sidebar-link-collapsed': sidebarCollapsed,
              }"
              :title="sidebarCollapsed ? item.label : undefined"
              :id="
                item.path === '/admin/providers'
                  ? 'sidebar-channel-manage'
                  : item.path === '/admin/groups'
                    ? 'sidebar-group-manage'
                    : item.path === '/admin/redeem'
                      ? 'sidebar-wallet'
                      : undefined
              "
              @click="handleMenuItemClick(item.path)"
            >
              <span
                v-if="item.iconSvg"
                class="flex-shrink-0 sidebar-svg-icon"
                v-html="sanitizeSvg(item.iconSvg)"
              ></span>
              <Icon v-else :name="item.icon ?? 'home'" size="sm" class="h-4 w-4 flex-shrink-0" />
              <span
                class="sidebar-label"
                :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
                :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
                >{{ item.label }}</span
              >
            </router-link>
          </template>
        </div>

        <!-- Personal account destinations for administrators. -->
        <div class="sidebar-section">
          <div
            class="sidebar-section-title mb-1 px-2.5 text-xs font-medium tracking-[0.04em] text-gray-500/85 dark:text-dark-400/85"
            :class="{ 'sidebar-section-title-collapsed': sidebarCollapsed }"
            :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
          >
            <span
              class="sidebar-section-title-text"
              :class="{ 'sidebar-section-title-text-collapsed': sidebarCollapsed }"
            >
              {{ t('nav.myAccount') }}
            </span>
          </div>

          <router-link
            v-for="item in accountNavItems"
            :key="item.path"
            :to="item.path"
            class="sidebar-link mb-0.5"
            :class="{
              'sidebar-link-active': isActive(item.path),
              'sidebar-link-collapsed': sidebarCollapsed,
            }"
            :title="sidebarCollapsed ? item.label : undefined"
            :data-tour="
              item.path === '/keys'
                ? 'sidebar-my-keys'
                : item.path === '/usage'
                  ? 'sidebar-usage'
                  : undefined
            "
            @click="handleMenuItemClick(item.path)"
          >
            <span
              v-if="item.iconSvg"
              class="flex-shrink-0 sidebar-svg-icon"
              v-html="sanitizeSvg(item.iconSvg)"
            ></span>
            <Icon v-else :name="item.icon ?? 'home'" size="sm" class="h-4 w-4 flex-shrink-0" />
            <span
              class="sidebar-label"
              :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
              :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
              >{{ item.label }}</span
            >
          </router-link>
        </div>
      </template>

      <!-- Regular User View -->
      <template v-else-if="!appStore.backendModeEnabled">
        <div class="sidebar-section">
          <router-link
            v-for="item in accountNavItems"
            :key="item.path"
            :to="item.path"
            class="sidebar-link mb-0.5"
            :class="{
              'sidebar-link-active': isActive(item.path),
              'sidebar-link-collapsed': sidebarCollapsed,
            }"
            :title="sidebarCollapsed ? item.label : undefined"
            :data-tour="
              item.path === '/keys'
                ? 'sidebar-my-keys'
                : item.path === '/usage'
                  ? 'sidebar-usage'
                  : undefined
            "
            @click="handleMenuItemClick(item.path)"
          >
            <span
              v-if="item.iconSvg"
              class="flex-shrink-0 sidebar-svg-icon"
              v-html="sanitizeSvg(item.iconSvg)"
            ></span>
            <Icon v-else :name="item.icon ?? 'home'" size="sm" class="h-4 w-4 flex-shrink-0" />
            <span
              class="sidebar-label"
              :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
              :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
              >{{ item.label }}</span
            >
          </router-link>
        </div>
      </template>
    </nav>

    <!-- Pinned Codex bottom account footer: avatar, balance, theme toggle, and upward account menu. -->
    <div
      v-if="user"
      ref="accountMenuRef"
      class="relative shrink-0 border-t border-black/[0.07] p-2 dark:border-white/[0.08]"
    >
      <div
        class="flex items-center gap-1.5"
        :class="sidebarCollapsed ? 'flex-col justify-center' : 'justify-between'"
      >
        <button
          type="button"
          class="flex min-w-0 flex-1 items-center gap-2.5 rounded-control px-1.5 py-1.5 text-left transition-colors duration-fast hover:bg-black/[0.045] dark:hover:bg-white/[0.06]"
          :class="sidebarCollapsed ? 'justify-center px-0' : ''"
          :title="sidebarCollapsed ? `${displayName} · ${formattedBalance}` : undefined"
          :aria-expanded="accountMenuOpen"
          :aria-label="t('common.userMenu')"
          @click="accountMenuOpen = !accountMenuOpen"
        >
          <UserAvatar
            :avatar-url="avatarUrl"
            :user-id="user.id"
            :alt="displayName"
            size-class="h-7 w-7 shrink-0"
          />
          <span v-if="!sidebarCollapsed" class="min-w-0 flex-1">
            <span class="block truncate text-xs font-medium leading-tight text-gray-950 dark:text-white">
              {{ displayName }}
            </span>
            <span class="mt-0.5 block truncate font-mono text-xs tabular-nums text-gray-500 dark:text-dark-400">
              {{ formattedBalance }}
            </span>
          </span>
        </button>

        <button
          type="button"
          class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-control text-gray-500 transition-colors duration-fast hover:bg-black/[0.05] hover:text-gray-950 dark:text-dark-400 dark:hover:bg-white/[0.07] dark:hover:text-white"
          :aria-label="isDark ? t('nav.lightMode') : t('nav.darkMode')"
          :title="isDark ? t('nav.lightMode') : t('nav.darkMode')"
          @click="toggleTheme"
        >
          <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
        </button>
      </div>

      <MotionTransition name="dropdown-fade">
        <div
          v-if="accountMenuOpen"
          class="dropdown user-menu-panel bottom-full left-2 right-2 z-modal mb-2 w-auto min-w-[220px] origin-bottom-left py-0"
        >
          <div class="menu-section">
            <router-link
              to="/profile"
              class="user-menu-card"
              @click="accountMenuOpen = false"
            >
              <UserAvatar
                :avatar-url="avatarUrl"
                :user-id="user.id"
                :alt="displayName"
                size-class="h-8 w-8 shrink-0"
              />
              <span class="min-w-0 flex-1">
                <span class="block truncate text-xs font-medium text-gray-950 dark:text-white">
                  {{ displayName }}
                </span>
                <span class="block truncate text-xs text-gray-500 dark:text-dark-400">
                  {{ user.email }}
                </span>
              </span>
              <Icon name="cog" size="xs" class="shrink-0 text-gray-400 dark:text-dark-400" />
            </router-link>
          </div>

          <div class="menu-section">
            <router-link
              to="/subscriptions"
              class="menu-item text-xs"
              @click="accountMenuOpen = false"
            >
              <Icon name="creditCard" size="sm" class="shrink-0" />
              <span class="flex-1">{{ t('nav.mySubscriptions') }}</span>
              <span class="font-mono text-xs tabular-nums text-gray-500 dark:text-dark-400">
                {{ formattedBalance }}
              </span>
            </router-link>
            <router-link
              to="/keys"
              class="menu-item text-xs"
              @click="accountMenuOpen = false"
            >
              <Icon name="key" size="sm" class="shrink-0" />
              {{ t('nav.apiKeys') }}
            </router-link>
          </div>

          <!-- Compact palette swatches inside the Codex account popover. -->
          <div class="menu-section">
            <div class="flex items-center justify-between px-2.5 py-1.5">
              <span class="text-xs text-gray-500 dark:text-dark-400">
                {{ isZhLocale ? '色板' : 'Palette' }}
              </span>
              <div class="flex items-center gap-1.5">
                <button
                  v-for="item in paletteOptions"
                  :key="item.id"
                  type="button"
                  :title="item.label"
                  :aria-pressed="themePalette === item.id"
                  class="h-3.5 w-3.5 rounded-full transition-transform duration-fast hover:scale-110"
                  :class="
                    themePalette === item.id
                      ? 'ring-2 ring-gray-900 ring-offset-2 ring-offset-white dark:ring-white dark:ring-offset-dark-900'
                      : 'opacity-65 hover:opacity-100'
                  "
                  :style="{ backgroundColor: item.swatch }"
                  @click="setThemePalette(item.id)"
                />
              </div>
            </div>
          </div>

          <div class="menu-section">
            <button
              type="button"
              class="menu-item menu-item-danger text-xs"
              @click="handleLogout"
            >
              <Icon name="logout" size="sm" />
              {{ t('nav.logout') }}
            </button>
          </div>
        </div>
      </MotionTransition>
    </div>
  </aside>

  <!-- Full-height Mobile Overlay -->
  <MotionTransition name="fade">
    <div
      v-if="mobileOpen"
      class="mobile-overlay fixed inset-0 z-sidebar-overlay bg-black/50 lg:hidden"
      @click="closeMobile"
    ></div>
  </MotionTransition>
</template>

<script setup lang="ts">
import MotionTransition from '@/components/common/MotionTransition.vue'
import Collapse from '@/components/common/Collapse.vue'
import UserAvatar from '@/components/common/UserAvatar.vue'
import VersionBadge from '@/components/admin/system/VersionBadge.vue'

import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAdminSettingsStore, useAppStore, useAuthStore, useOnboardingStore } from '@/stores'
import { sanitizeSvg } from '@/utils/sanitize'
import { sanitizeUrl } from '@/utils/url'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import { useTheme, type ThemePalette } from '@/composables/useTheme'
import { useNavigationItems, type NavItem } from '@/components/layout/navigationItems'

// AppSidebar.vue renders the full-height Codex navigation rail with brand and collapse
// controls at the top, semantically grouped role destinations in the scrollable body,
// and a pinned account/balance/theme footer at the bottom.

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const onboardingStore = useOnboardingStore()
const adminSettingsStore = useAdminSettingsStore()
const { accountNavItems, adminNavItems } = useNavigationItems()
const { formatBalanceAmount, balanceUnitSymbol } = useBalanceDisplay()
const { isDark, themePalette, setThemePalette, toggleTheme } = useTheme()

const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const mobileOpen = computed(() => appStore.mobileOpen)
const isAdmin = computed(() => authStore.isAdmin)
const user = computed(() => authStore.user)
const isZhLocale = computed(() => String(locale.value || '').toLowerCase().startsWith('zh'))

const siteName = computed(() => appStore.siteName)
const siteLogo = computed(() =>
  sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }),
)
const siteVersion = computed(() => appStore.siteVersion)

const displayName = computed(() => {
  const current = user.value
  if (!current) return ''
  return current.username?.trim() || current.email || ''
})
const avatarUrl = computed(() =>
  sanitizeUrl(user.value?.avatar_url || '', { allowRelative: true, allowDataUrl: true }),
)
const formattedBalance = computed(() => {
  const amount = Math.max(0, Number(user.value?.balance ?? 0))
  const text = formatBalanceAmount(amount, true)
  return text.startsWith(balanceUnitSymbol.value) ? text : `${balanceUnitSymbol.value}${text}`
})

const paletteOptions: Array<{ id: ThemePalette; label: string; swatch: string }> = [
  { id: 'terracotta', label: '暖玉', swatch: '#C46D45' },
  { id: 'pearl', label: '珍珠', swatch: '#52525B' },
  { id: 'indigo', label: '雾靛', swatch: '#5B6CF9' },
]

// Semantic Codex section buckets keep the 20+ admin entries scannable without dropping any route.
const OVERVIEW_PATHS = new Set(['/admin/dashboard', '/admin/ops'])
const RESOURCE_PATHS = new Set([
  '/admin/providers',
  '/admin/groups',
  '/admin/model-management',
  '/admin/proxies',
  '/admin/mcp',
  '/admin/plugins',
])
const COMMERCE_PATHS = new Set([
  '/admin/users',
  '/admin/teams',
  '/admin/subscriptions',
  '/admin/orders',
  '/admin/redeem',
  '/admin/promo-codes',
  '/admin/affiliates',
  '/admin/risk-control',
  '/admin/announcements',
])

const groupedAdminSections = computed(() => {
  const overview: NavItem[] = []
  const resources: NavItem[] = []
  const commerce: NavItem[] = []
  const system: NavItem[] = []

  for (const item of adminNavItems.value) {
    if (OVERVIEW_PATHS.has(item.path)) overview.push(item)
    else if (RESOURCE_PATHS.has(item.path)) resources.push(item)
    else if (COMMERCE_PATHS.has(item.path)) commerce.push(item)
    else system.push(item)
  }

  const zh = isZhLocale.value
  return [
    { key: 'overview', label: zh ? '概览' : 'Overview', items: overview },
    { key: 'resources', label: zh ? '资源与路由' : 'Routing & Models', items: resources },
    { key: 'commerce', label: zh ? '商业与风控' : 'Users & Billing', items: commerce },
    { key: 'system', label: zh ? '审计与系统' : 'Audit & System', items: system },
  ].filter((section) => section.items.length > 0)
})

const sidebarNavRef = ref<HTMLElement | null>(null)
const accountMenuRef = ref<HTMLElement | null>(null)
const accountMenuOpen = ref(false)
const groupExpandOverrides = ref<Map<string, boolean>>(new Map())

watch(
  () => route.fullPath,
  () => {
    if (mobileOpen.value) appStore.setMobileOpen(false)
    accountMenuOpen.value = false
  },
)

function closeMobile() {
  appStore.setMobileOpen(false)
}

async function handleLogout() {
  accountMenuOpen.value = false
  await authStore.logout()
  await router.push('/login')
}

function handleDocumentClick(event: MouseEvent) {
  if (!accountMenuOpen.value) return
  const target = event.target as Node | null
  if (accountMenuRef.value && target && !accountMenuRef.value.contains(target)) {
    accountMenuOpen.value = false
  }
}

function handleMenuItemClick(itemPath: string) {
  if (mobileOpen.value) {
    appStore.setMobileOpen(false)
  }

  const pathToSelector: Record<string, string> = {
    '/admin/groups': '#sidebar-group-manage',
    '/admin/providers': '#sidebar-channel-manage',
    '/keys': '[data-tour="sidebar-my-keys"]',
    '/usage': '[data-tour="sidebar-usage"]',
  }

  const selector = pathToSelector[itemPath]
  if (selector && onboardingStore.isCurrentStep(selector)) {
    onboardingStore.nextStep(500)
  }
}

function isActive(path: string): boolean {
  return route.path === path || route.path.startsWith(path + '/')
}

function isGroupActive(item: NavItem): boolean {
  if (!item.children) return false
  return item.children.some((child) => route.path === child.path)
}

function isGroupExpanded(item: NavItem): boolean {
  const override = groupExpandOverrides.value.get(item.path)
  if (override !== undefined) return override
  return isGroupActive(item)
}

function toggleGroup(item: NavItem) {
  groupExpandOverrides.value.set(item.path, !isGroupExpanded(item))
}

watch(
  isAdmin,
  (v) => {
    if (v) {
      adminSettingsStore.fetch()
    }
  },
  { immediate: true },
)

onMounted(() => {
  document.addEventListener('click', handleDocumentClick)
  if (isAdmin.value) {
    adminSettingsStore.fetch()
  }
  if (appStore.sidebarScrollTop > 0 && sidebarNavRef.value) {
    void nextTick(() => {
      if (sidebarNavRef.value) {
        sidebarNavRef.value.scrollTop = appStore.sidebarScrollTop
      }
    })
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleDocumentClick)
  if (sidebarNavRef.value) {
    appStore.sidebarScrollTop = sidebarNavRef.value.scrollTop
  }
})
</script>

<style scoped>
.sidebar-link-collapsed {
  gap: 0;
  justify-content: center;
  padding-left: 0;
  padding-right: 0;
}

.sidebar-section-title {
  position: relative;
  display: flex;
  align-items: center;
  min-height: 1.25rem;
  overflow: hidden;
  white-space: nowrap;
}

.sidebar-section-title-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    opacity var(--motion-fast) var(--motion-ease),
    transform var(--motion-fast) var(--motion-ease);
}

.sidebar-section-title::after {
  content: '';
  position: absolute;
  left: 0.75rem;
  right: 0.75rem;
  top: 50%;
  height: 1px;
  background: rgb(229 231 235);
  opacity: 0;
  transform: translateY(-50%);
  transition: opacity var(--motion-fast) var(--motion-ease);
}

.dark .sidebar-section-title::after {
  background: rgb(255 255 255 / 0.1);
}

.sidebar-section-title-text-collapsed {
  opacity: 0;
  transform: translateX(-4px);
}

.sidebar-section-title-collapsed::after {
  opacity: 1;
}

.sidebar-label {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    max-width var(--motion-layout) var(--motion-ease),
    opacity var(--motion-fast) var(--motion-ease),
    transform var(--motion-fast) var(--motion-ease);
  max-width: 12rem;
}

.sidebar-label-flex {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.sidebar-label-collapsed {
  max-width: 0;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}

.sidebar-svg-icon {
  width: 1rem;
  height: 1rem;
  color: currentColor;
}

.sidebar-svg-icon :deep(svg) {
  display: block;
  width: 100%;
  height: 100%;
}
</style>
