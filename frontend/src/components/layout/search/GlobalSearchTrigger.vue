<template>
  <!--
    The root carries the field width so a crowded header can shrink it before the
    navigation beside it; min-w-32 keeps it recognizable as a field.
  -->
  <div class="flex min-w-0 shrink items-center sm:w-44 sm:min-w-32 md:w-48 lg:w-60 xl:w-72">
    <!-- The narrow search button shares the color and hover behavior of the navigation icons. -->
    <button
      type="button"
      class="header-nav-link sm:hidden"
      data-testid="global-search-icon-trigger"
      aria-haspopup="dialog"
      :aria-expanded="globalSearchOpen"
      :aria-label="t('globalSearch.open')"
      :title="t('globalSearch.open')"
      @click="openGlobalSearch"
    >
      <Icon name="search" size="md" />
    </button>

    <!--
      From sm upward the trigger looks like a search field so its purpose is readable
      at a glance; it is still a button because typing happens in the palette.
    -->
    <button
      type="button"
      class="global-search-trigger hidden w-full sm:flex"
      data-testid="global-search-trigger"
      aria-haspopup="dialog"
      :aria-expanded="globalSearchOpen"
      :aria-keyshortcuts="ariaShortcut"
      @click="openGlobalSearch"
    >
      <Icon name="search" size="sm" class="shrink-0" />
      <span class="min-w-0 flex-1 truncate text-left">{{ t('globalSearch.placeholder') }}</span>
      <kbd class="kbd ml-auto hidden lg:inline-flex" data-testid="global-search-kbd">
        {{ shortcutLabel }}
      </kbd>
    </button>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import {
  globalSearchOpen,
  openGlobalSearch,
  registerSearchTrigger,
} from '@/components/layout/search/globalSearchState'
import { isApplePlatform, modifierLabel } from '@/components/layout/search/searchShortcuts'

// GlobalSearchTrigger.vue renders the header entry for the global search palette: a
// search-field look-alike from sm upward and an icon button below sm. Clicking either
// opens the palette mounted by App.vue. While mounted, the trigger registers itself so
// the palette enables its keyboard shortcuts only on pages that show a search entry.

// t translates the placeholder and the icon button's accessible name.
const { t } = useI18n()

// shortcutLabel shows the platform's shortcut ("⌘K" or "Ctrl K"), and ariaShortcut
// announces the same keys in the aria-keyshortcuts syntax.
const shortcutLabel = isApplePlatform() ? `${modifierLabel()}K` : `${modifierLabel()} K`
const ariaShortcut = isApplePlatform() ? 'Meta+K' : 'Control+K'

// releaseTrigger is the registration release returned on mount, called once on unmount.
let releaseTrigger: (() => void) | null = null

// Registration starts on mount, which always has a matching unmount, so a component
// whose setup never completes leaves no registration behind.
onMounted(() => {
  releaseTrigger = registerSearchTrigger()
})

// Releasing on unmount turns the shortcuts off once no header shows a search entry.
onBeforeUnmount(() => {
  releaseTrigger?.()
  releaseTrigger = null
})
</script>

<style scoped>
/*
 * The trigger follows the 36px control baseline and the input border tokens, with a
 * slightly tinted fill so it reads as a field on the translucent header. The shortcut
 * hint appears from lg upward, where the field is wide enough to keep the placeholder.
 */
.global-search-trigger {
  @apply h-9 min-w-0 items-center gap-2 rounded-full border px-3.5 text-sm;
  @apply border-black/[0.08] bg-white/60 text-gray-500 backdrop-blur-md;
  @apply transition-colors duration-fast;
  @apply hover:border-black/[0.15] hover:bg-white/85 hover:text-gray-700;
  @apply focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black/10;
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.7);
}

.dark .global-search-trigger {
  @apply border-white/[0.09] bg-white/[0.05] text-dark-400 backdrop-blur-md;
  @apply hover:border-white/[0.16] hover:bg-white/[0.08] hover:text-dark-200 focus-visible:ring-white/10;
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06);
}
</style>
