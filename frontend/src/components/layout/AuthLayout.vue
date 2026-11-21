<template>
  <div class="ba-theme-shell relative min-h-screen text-gray-900 dark:text-dark-50">
    <!-- Non-home architectural topography backdrop with slow ambient motion and a soft glow. -->
    <LazyVisual class="pointer-events-none fixed inset-0 overflow-hidden bg-[var(--page-bg)]">
      <Topography
        :color="topographyColor"
        :line-count="18"
        :line-width="1.05"
        :major-lines="true"
        :elevation="0.85"
        :foreground="0.18"
        :scale="1.15"
        :detail="0.3"
        :tilt="34"
        :speed="0.25"
        direction="forward"
        :morph="0.2"
        :glow="0.6"
        :depth-of-field="0.4"
        :fog="0.4"
        :brightness="isDark ? 0.85 : 0.95"
        :opacity="isDark ? 0.36 : 0.32"
        :mouse-interaction="true"
        hover-effect="lift"
        :mouse-radius="1.2"
        :mouse-strength="0.85"
        :trail="0.45"
        :parallax="0.65"
        click-effect="pulse"
        :intro="false"
        :light-mode="!isDark"
        :dpr="1"
        :max-fps="30"
        :max-cells="42000"
      />
    </LazyVisual>


    <!-- Minimal floating editorial header with brand, locale, and theme toggle -->
    <header
      class="site-header fixed inset-x-0 top-0 z-header border-b border-black/[0.06] dark:border-white/[0.07]"
    >
      <div class="flex h-[var(--header-h)] items-center justify-between px-6 lg:px-10">
        <router-link
          to="/home"
          class="inline-flex items-center gap-2.5 text-sm font-medium tracking-[-0.01em] text-gray-900 transition-colors hover:text-gray-600 dark:text-dark-50 dark:hover:text-dark-300"
        >
          <span
            class="flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded-control border border-white/80 bg-white/80 p-0.5 shadow-sm dark:border-white/[0.12] dark:bg-white/[0.08]"
          >
            <img
              :src="siteLogo || '/logo.svg'"
              :alt="siteName"
              class="h-full w-full object-contain"
            />
          </span>
          <span>{{ siteName }}</span>
        </router-link>


        <div class="flex items-center gap-2">
          <LocaleSwitcher variant="text" />
          <button
            type="button"
            data-testid="auth-theme-toggle"
            class="header-status-icon-button"
