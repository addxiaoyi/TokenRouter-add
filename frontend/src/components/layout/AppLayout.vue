<template>
  <div class="ba-theme-shell" :class="shellClass">
    <!-- Skip link lets keyboard users jump directly past fixed header and sidebar chrome. -->
    <a
      href="#main-content"
      class="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-3 focus:z-tooltip focus:rounded-control focus:bg-[#141413] focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-[#faf9f5] focus:shadow-lg dark:focus:bg-[#faf9f5] dark:focus:text-[#141413]"
    >
      Skip to content
    </a>


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
        :brightness="isDarkTheme ? 0.85 : 0.95"
        :opacity="isDarkTheme ? 0.36 : 0.32"
        :mouse-interaction="true"
        hover-effect="lift"
        :mouse-radius="1.2"
        :mouse-strength="0.85"
        :trail="0.45"
        :parallax="0.65"
        click-effect="pulse"
        :intro="false"
        :light-mode="!isDarkTheme"
        :dpr="1"
        :max-fps="24"
        :max-cells="24000"
      />
    </LazyVisual>


    <!--
      In console mode the contextual header spans the main column while the full-height
      Codex sidebar owns brand and account controls.
    -->
    <AppHeader :console-mode="!hideSidebar" />


    <!-- Sidebar and Main Content Area -->
    <AppSidebar v-if="!hideSidebar" />


    <div
      class="relative z-10 flex min-w-0 flex-col pt-[var(--header-h)] transition-[margin-left] duration-layout"
      :class="[
        columnClass,
        hideSidebar
          ? ''
          : sidebarCollapsed
            ? 'lg:ml-[var(--sidebar-w-collapsed)]'
            : 'lg:ml-[var(--sidebar-w)]',
      ]"
