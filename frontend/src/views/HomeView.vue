<template>
  <GoogleOneTap :enabled="googleOneTapEligible" :client-id="googleOneTapClientID" />

  <!-- Configured home content replaces the default landing page while retaining the independent One Tap control. -->
  <div v-if="homeContent" class="min-h-screen">
    <!-- HTTP(S) content is embedded directly as the configured full-page destination. -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- Administrator-configured HTML is inserted directly; this rendering path performs no additional HTML sanitization. -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- The built-in page supplies marketplace summaries when no custom home content is configured. -->
  <div
    v-else
    class="ba-theme-shell relative flex min-h-screen flex-col pt-[var(--header-h)] text-gray-950 dark:text-white"
  >
    <!-- Share the application header while reserving its fixed height in the public page shell. -->
    <AppHeader public-page />

    <main v-content-reveal="motionRoute?.path" class="relative z-10 flex-1 pb-24 md:pb-32">
      <!-- The hero renders immediately; lower artwork waits until its reserved section approaches the viewport. -->
      <section class="relative w-full overflow-hidden px-6 md:px-10">
        <div class="pointer-events-none absolute inset-0 z-0">
          <GradientASCII
            :colors="heroAsciiColors"
            shape="horizon"
            characters=" =+xX0S8#@"
            :cell-size="18"
            :density="0.86"
            :glyph-opacity="isDarkTheme ? 0.9 : 0.85"
            :gradient-opacity="isDarkTheme ? 0.9 : 0.82"
            :glow="0"
            :flicker="0.45"
            :softness="0.5"
            :scale="1.05"
            :flow="1.4"
            :speed="1.8"
            :grain="0.06"
            :mouse-interaction="true"
            :scramble-radius="0.18"
            click-effect="wave"
            :intro="true"
            :light-mode="!isDarkTheme"
            :dpr="1"
            :max-fps="30"
            font-family="'Geist Mono Variable', 'Geist Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
          />
          <!-- Soft bottom fade seamlessly transitions the hero light field into the gallery canvas below. -->
          <div
            class="absolute inset-x-0 bottom-0 h-28 bg-gradient-to-t from-[var(--page-bg)] to-transparent"
          ></div>
        </div>

        <div
          class="pointer-events-none relative z-10 mx-auto flex min-h-[calc(100vh-var(--header-h))] max-w-7xl flex-col justify-between py-12 lg:py-16"
        >
          <div class="my-auto flex max-w-2xl flex-col items-start py-8">
            <h1
              class="text-balance text-left text-5xl font-light leading-[1.04] tracking-[-0.04em] text-gray-950 sm:text-6xl lg:text-[4.5rem] dark:text-white"
            >
              {{ homeHeroTitle }}
            </h1>

            <p
              class="mt-6 max-w-[44ch] text-pretty text-left text-base leading-relaxed text-gray-900/75 sm:text-lg dark:text-dark-50/75"
            >
              {{ homeHeroSubtitle }}
            </p>

            <div
              class="pointer-events-auto mt-9 flex w-full flex-col gap-3 sm:w-auto sm:flex-row sm:items-center"
            >
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="inline-flex min-h-12 min-w-[176px] items-center justify-center gap-2.5 rounded-full bg-gray-950 px-7 py-3 text-sm font-medium text-gray-50 shadow-sm transition duration-fast ease-standard hover:-translate-y-0.5 hover:bg-gray-800 active:scale-[0.98] dark:bg-dark-50 dark:text-dark-950 dark:hover:bg-dark-200"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="sm" :stroke-width="2" />
              </router-link>
              <router-link
                to="/models"
                class="inline-flex min-h-12 min-w-[176px] items-center justify-center gap-2.5 rounded-full border border-black/[0.12] bg-white/85 px-7 py-3 text-sm font-medium text-gray-950 shadow-sm transition duration-fast ease-standard hover:-translate-y-0.5 hover:bg-white active:scale-[0.98] dark:border-white/[0.16] dark:bg-black/55 dark:text-dark-50 dark:hover:bg-black/75"
              >
                {{ t('home.exploreMarketplace') }}
                <span class="relative flex h-4 w-4 items-center justify-center overflow-hidden">
                  <MotionTransition name="home-marketplace-icon" mode="out-in">
                    <ProviderIcon
                      v-if="homeMarketplaceButtonBrand"
                      :key="homeMarketplaceButtonBrand"
                      :brand="homeMarketplaceButtonBrand"
                      size="16px"
                    />
                    <Icon
                      v-else
                      key="marketplace-fallback"
                      name="sparkles"
                      size="sm"
                      class="text-primary-500"
                    />
                  </MotionTransition>
                </span>
              </router-link>
            </div>
          </div>

          <div
            v-if="!homeStatsError"
            class="grid grid-cols-2 divide-y divide-black/[0.08] border-y border-black/[0.08] sm:grid-cols-4 sm:divide-x sm:divide-y-0 dark:divide-white/[0.10] dark:border-white/[0.10]"
          >
            <div
              v-for="card in homeStatsCards"
              :key="card.key"
              class="min-w-0 py-5 sm:px-5 sm:first:pl-0 sm:last:pr-0"
            >
              <p class="text-xs text-gray-900/65 dark:text-dark-50/65">
                {{ card.label }}
              </p>
              <p
                class="mt-1 truncate text-2xl font-light tabular-nums tracking-[-0.03em] text-gray-950 sm:text-3xl lg:text-4xl dark:text-white"
              >
                {{ card.value }}
              </p>
            </div>
          </div>
          <div
            v-else
            class="flex flex-wrap items-center gap-3 border-y border-black/[0.08] py-4 dark:border-white/[0.10]"
          >
            <span
              v-for="brand in ['anthropic', 'openai', 'gemini', 'deepseek', 'xai']"
              :key="brand"
              class="flex h-8 w-8 items-center justify-center rounded-control border border-white/80 bg-white/70 shadow-sm dark:border-white/[0.10] dark:bg-white/[0.06]"
            >
              <ProviderIcon :brand="brand" size="18px" />
            </span>
            <span class="text-xs text-gray-900/60 dark:text-dark-50/60">
              {{ t('home.stats.unavailable') }}
            </span>
          </div>
        </div>
      </section>

      <div class="px-6 md:px-10">
        <!-- Section 2: Single uncluttered editorial band pairing interactive DitherVeil with borderless hairline feature rows (zero stacked card bloat) -->
        <section
          class="mx-auto mt-20 grid max-w-7xl grid-cols-1 items-center gap-12 lg:mt-28 lg:grid-cols-12"
        >
          <div class="lg:col-span-6">
            <div
              class="group relative isolate h-[380px] w-full overflow-hidden rounded-surface border border-black/[0.08] bg-gray-950 shadow-xl sm:h-[440px] dark:border-white/[0.12]"
            >
              <LazyVisual class="absolute inset-0">
                <template #fallback>
                  <img
                    :src="heroDitherArt"
                    alt=""
                    loading="lazy"
                    decoding="async"
                    class="h-full w-full object-cover grayscale"
                  />
                </template>
                <DitherVeil
                  :src="heroDitherArt"
                  fit="cover"
                  pattern="floyd"
                  :pixel-size="2"
                  :levels="2"
                  palette="duotone"
                  :ink-color="heroVeilTheme.ink"
                  :paper-color="heroVeilTheme.paper"
                  :rim-color="heroVeilTheme.rim"
                  :rim="0.14"
                  :contrast="1.18"
                  :reveal-radius="210"
                  :softness="0.6"
                  :linger="1.15"
                  :wander="false"
                  :click-burst="true"
                />
              </LazyVisual>

              <div class="pointer-events-none absolute inset-x-4 top-4 z-10 sm:inset-x-5 sm:top-5">
                <span
                  class="inline-flex items-center gap-2 rounded-full border border-white/25 bg-black/75 px-3 py-1 font-mono text-xs text-white/95"
                >
                  <span class="h-1.5 w-1.5 rounded-full bg-emerald-400"></span>
                  {{ homeRouteLabel }}
                </span>
              </div>
            </div>
          </div>

          <div
            class="divide-y divide-black/[0.08] border-y border-black/[0.08] lg:col-span-6 dark:divide-white/[0.10] dark:border-white/[0.10]"
          >
            <div class="py-6">
              <div class="flex items-baseline justify-between gap-4">
                <h2 class="text-xl font-light tracking-[-0.02em] text-gray-950 dark:text-white">
                  {{ t('home.features.unifiedGateway') }}
                </h2>
                <router-link
                  to="/models"
                  class="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-link"
                >
                  {{ t('home.features.browseAll') }}
                  <Icon name="arrowRight" size="xs" />
                </router-link>
              </div>
              <p class="mt-2 text-sm leading-relaxed text-gray-900/65 dark:text-dark-50/65">
                {{ t('home.features.unifiedGatewayDesc') }}
              </p>
            </div>

            <div class="py-6">
              <div class="flex items-baseline justify-between gap-4">
                <h2 class="text-xl font-light tracking-[-0.02em] text-gray-950 dark:text-white">
                  {{ t('home.features.multiProvider') }}
                </h2>
                <router-link
                  :to="isAuthenticated ? dashboardPath : '/login'"
                  class="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-link"
                >
                  {{ t('home.features.learnMore') }}
                  <Icon name="arrowRight" size="xs" />
                </router-link>
              </div>
              <p class="mt-2 text-sm leading-relaxed text-gray-900/65 dark:text-dark-50/65">
                {{ t('home.features.multiProviderDesc') }}
              </p>
            </div>

            <div class="py-6">
              <div class="flex items-baseline justify-between gap-4">
                <h2 class="text-xl font-light tracking-[-0.02em] text-gray-950 dark:text-white">
                  {{ t('home.features.balanceQuota') }}
                </h2>
                <router-link
                  :to="isAuthenticated ? dashboardPath : '/login'"
                  class="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-link"
                >
                  {{ t('home.features.viewUsage') }}
                  <Icon name="arrowRight" size="xs" />
                </router-link>
              </div>
              <p class="mt-2 text-sm leading-relaxed text-gray-900/65 dark:text-dark-50/65">
                {{ t('home.features.balanceQuotaDesc') }}
              </p>
            </div>

            <div class="py-6">
              <div class="flex items-baseline justify-between gap-4">
                <h2 class="text-xl font-light tracking-[-0.02em] text-gray-950 dark:text-white">
                  {{ t('home.features.dataPolicies') }}
                </h2>
                <a
                  v-if="docUrl"
                  :href="docUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-link"
                >
                  {{ t('home.docs') }}
                  <Icon name="externalLink" size="xs" />
                </a>
              </div>
              <p class="mt-2 text-sm leading-relaxed text-gray-900/65 dark:text-dark-50/65">
                {{ t('home.features.dataPoliciesDesc') }}
              </p>
            </div>
          </div>
        </section>

        <section class="mx-auto mt-24 max-w-7xl">
          <div class="mb-7 flex items-end justify-between gap-4">
            <div class="min-w-0">
              <h2
                class="text-balance text-xl font-semibold leading-tight tracking-[-0.025em] text-gray-950 dark:text-white"
              >
                {{ t('home.providers.title') }}
              </h2>
              <p class="mt-1.5 text-sm leading-relaxed text-gray-500 dark:text-dark-400">
                {{ formatMarketplaceStat(totalModelCount) }} {{ t('marketplace.modelsStat') }}
                ·
                {{ formatMarketplaceStat(supportedProviders.length) }}
                {{ t('home.stats.providerTypes') }}
              </p>
            </div>
            <router-link
              to="/models"
              class="inline-flex shrink-0 items-center gap-1 whitespace-nowrap text-sm leading-relaxed text-link"
            >
              {{ t('home.viewAll') }}
              <Icon name="arrowRight" size="xs" />
            </router-link>
          </div>

          <div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
            <template v-if="homeMarketplaceLoading">
              <span role="status" :aria-label="t('common.loading')" class="sr-only"></span>
              <article
                v-for="card in 3"
                :key="card"
                class="rounded-surface border border-gray-200 bg-white p-6 dark:border-dark-800 dark:bg-dark-900"
                aria-hidden="true"
                aria-busy="true"
                data-loading-skeleton
              >
                <div class="flex items-start gap-4">
                  <Skeleton :width="48" :height="48" class="shrink-0 rounded-surface" />
                  <div class="min-w-0 flex-1 space-y-2 py-1">
                    <Skeleton :width="146" :height="20" class="max-w-full" />
                    <Skeleton :width="92" :height="14" class="max-w-full" />
                  </div>
                </div>
                <div
                  class="mt-5 grid grid-cols-2 gap-1 border-t border-gray-200 pt-5 dark:border-dark-800 sm:gap-4"
                >
                  <div v-for="field in 2" :key="field" class="space-y-2">
                    <Skeleton width="70%" :height="14" />
                    <Skeleton width="48%" :height="14" />
                  </div>
                </div>
                <Skeleton :width="96" :height="16" class="mt-5" />
              </article>
            </template>

            <div
              v-else-if="supportedProviders.length === 0"
              class="rounded-surface border border-gray-200 bg-white px-5 py-4 text-center text-sm text-gray-500 dark:border-dark-800 dark:bg-dark-900 dark:text-dark-400 sm:col-span-2 lg:col-span-3"
            >
              {{
                homeMarketplaceError ? t('home.providers.unavailable') : t('home.providers.empty')
              }}
            </div>

            <!-- Resolved featured-model configuration replaces the fallback provider-category cards in its configured order. -->
            <template v-else-if="featuredModels.length > 0">
              <article
                v-for="featured in featuredModels"
                :key="featured.model.id"
                class="glass-card card-hover p-6 focus-within:border-black/[0.16]"
              >
                <div class="flex items-start gap-4">
                  <!-- Use the same model artwork and framed icon treatment as the marketplace cards. -->
                  <span
                    class="flex h-12 w-12 shrink-0 items-center justify-center rounded-surface border border-black/[0.08] bg-white/85 shadow-sm dark:border-white/[0.09] dark:bg-dark-950/85"
                  >
                    <ModelIcon :model="featured.model.id" size="28px" />
                  </span>
                  <div class="min-w-0 flex-1">
                    <h3
                      class="truncate text-lg font-semibold tracking-[-0.02em] text-gray-950 dark:text-white"
                    >
                      {{ featured.model.display_name || featured.model.id }}
                    </h3>
                    <p
                      v-if="featured.author"
                      class="truncate text-sm text-gray-500 dark:text-dark-400"
                    >
                      {{ t('home.featured.byAuthor', { author: featured.author }) }}
                    </p>
                  </div>
                </div>
                <dl
                  class="mt-5 grid grid-cols-2 gap-1 border-t border-black/[0.07] pt-5 text-sm leading-relaxed dark:border-white/[0.08] sm:gap-4"
                >
                  <div class="min-w-0">
                    <dt class="text-gray-600 dark:text-dark-300">{{
                      t('home.featured.modalities')
                    }}</dt>
                    <dd class="flex min-h-[1.625em] items-center">
                      <ModelCapabilityTags
                        :model="featured.model"
                        class="home-featured-modalities"
                      />
                    </dd>
                  </div>
                  <div class="min-w-0 text-right">
                    <dt class="text-gray-600 dark:text-dark-300">{{
                      t('home.featured.maxDiscount')
                    }}</dt>
                    <dd
                      class="font-medium tabular-nums"
                      :class="
                        featured.discountOff
                          ? 'text-gray-950 dark:text-white'
                          : 'text-gray-500 dark:text-dark-400'
                      "
                    >
                      {{ featured.discountOff ?? '—' }}
                    </dd>
                  </div>
                </dl>
              </article>
            </template>

            <template v-else>
              <article
                v-for="provider in supportedProviders.slice(0, 6)"
                :key="provider.key"
                class="glass-card card-hover p-6 focus-within:border-black/[0.16]"
              >
                <div class="flex items-start gap-4">
                  <span
                    class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full border border-black/[0.08] bg-white/85 shadow-sm dark:border-white/[0.09] dark:bg-dark-950/85"
                    :class="providerIconWrapClass(provider)"
                  >
                    <ProviderIcon :brand="provider.iconBrand" size="22px" />
                  </span>
                  <div class="min-w-0 flex-1">
                    <h3
                      class="truncate text-lg font-semibold tracking-[-0.02em] text-gray-950 dark:text-white"
                    >
                      {{ provider.label }}
                    </h3>
                    <p class="text-sm text-gray-500 dark:text-dark-400">
                      {{ provider.groupCount }} {{ t('home.providers.groups') }}
                    </p>
                  </div>
                </div>
                <div class="mt-5 border-t border-black/[0.07] pt-5 dark:border-white/[0.08]">
                  <div class="flex items-end justify-between gap-4">
                    <div>
                      <p class="text-sm text-gray-500 dark:text-dark-400">
                        {{ t('home.providers.modelCount') }}
                      </p>
                      <p class="mt-1 text-lg font-semibold text-gray-950 dark:text-white">
                        {{ provider.modelCount }}
                      </p>
                    </div>
                    <p
                      v-if="provider.officialPriceRatio"
                      class="max-w-[180px] text-right text-sm font-semibold text-emerald-600 dark:text-emerald-300"
                    >
                      {{ formatOfficialPriceRatio(provider.officialPriceRatio) }}
                    </p>
                    <p v-else class="text-sm font-medium text-primary-600 dark:text-primary-300">
                      {{ t('home.providers.supported') }}
                    </p>
                  </div>
                </div>
              </article>
            </template>
          </div>
        </section>

        <!-- Interactive FallingText physics band showcasing multi-provider routing with Matter.js rigid bodies. -->
        <section class="mx-auto mt-16 max-w-7xl md:mt-20">
          <div class="glass-card relative overflow-hidden">
            <div class="pointer-events-none absolute inset-x-5 top-4 z-10">
              <span class="font-mono text-xs text-gray-500 dark:text-dark-400">
                Hover · Drag · Throw
              </span>
            </div>

            <div class="h-[250px] w-full pt-8 sm:h-[270px]">
              <LazyVisual class="relative">
                <template #fallback>
                  <p
                    class="flex h-full items-center justify-center px-6 text-center text-base text-gray-900 dark:text-dark-100"
                  >
                    One unified API routes Claude GPT Gemini DeepSeek Grok with automatic failover
                    and zero lock-in.
                  </p>
                </template>
                <FallingText
                  text="One unified API routes Claude GPT Gemini DeepSeek Grok with automatic failover and zero lock-in."
                  :highlight-words="['Claude', 'GPT', 'Gemini', 'DeepSeek', 'Grok']"
                  variant="tiles"
                  trigger="hover"
                  :gravity="0.95"
                  :bounciness="0.32"
                  :scatter="0.55"
                  :cursor-push="0.65"
                  :restore-delay="2.5"
                  font-size="1.05rem"
                  :accent-color="fallingAccentColor"
                  :theme="isDarkTheme ? 'dark' : 'light'"
                  class-name="text-gray-900 dark:text-dark-100"
                />
              </LazyVisual>
            </div>
          </div>
        </section>

        <section
          aria-labelledby="home-steps-heading"
          class="mx-auto mt-20 max-w-7xl text-sm leading-[1.65] text-gray-950 dark:text-dark-50 md:mt-24"
        >
          <h2 id="home-steps-heading" class="sr-only">{{ t('home.steps.title') }}</h2>
          <div class="grid gap-5 md:grid-cols-3">
            <article
              v-for="step in homeSteps"
              :key="step.key"
              class="glass-card flex min-w-0 flex-col gap-4 p-6"
            >
              <div class="flex items-center gap-3">
                <span
                  class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-black/[0.08] bg-white/80 font-mono text-xs font-semibold tabular-nums text-gray-700 shadow-sm dark:border-white/[0.10] dark:bg-white/[0.06] dark:text-dark-200"
                >
                  {{ step.index }}
                </span>
                <h3 class="text-base font-semibold leading-[1.35] tracking-[-0.015em]">
                  {{ step.title }}
                </h3>
              </div>

              <p class="min-h-10 text-pretty text-gray-600 dark:text-dark-300 md:min-h-12">
                {{ step.description }}
                <template v-if="step.key === 'api-key' && docUrl">
                  {{ ' ' }}
                  <a
                    :href="docUrl"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="text-link underline-offset-2 decoration-current/40"
                  >
                    {{ t('home.steps.apiKey.docs') }}
                  </a>
                </template>
              </p>

              <div class="flex w-full max-w-56 flex-col gap-3 pt-4 md:px-2" aria-hidden="true">
                <div class="flex items-center gap-2">
                  <svg
                    class="h-5 w-5 shrink-0"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  >
                    <path :d="onboardingPaths[step.icon]" />
                  </svg>
                  <span
                    v-for="(width, index) in step.illustrationLineWidths"
                    :key="index"
                    class="home-step-lines flex h-4 flex-col justify-center gap-0.5"
                    :class="width"
                  ></span>
                  <span
                    v-if="step.key === 'api-key'"
                    class="home-step-sample flex h-6 min-w-0 flex-1 items-center bg-gray-950/5 px-2 text-xs leading-[1.35] tracking-wide text-gray-950/[0.69] dark:bg-dark-50/5 dark:text-dark-50/[0.69]"
                  >
                    <span class="truncate">TOKENROUTER_API_KEY</span>
                  </span>
                </div>

                <div v-if="step.key === 'signup'" class="flex gap-2">
                  <span
                    class="flex h-10 w-10 items-center justify-center rounded-control bg-gray-950/[0.03] dark:bg-dark-50/[0.03]"
                  >
                    <GoogleMark class="h-5 w-5" />
                  </span>
                  <span
                    class="flex h-10 w-10 items-center justify-center rounded-control bg-gray-950/[0.03] dark:bg-dark-50/[0.03]"
                  >
                    <GitHubMark class="h-5 w-5" />
                  </span>
                  <span
                    class="flex h-10 w-10 items-center justify-center rounded-control bg-gray-950/[0.03] dark:bg-dark-50/[0.03]"
                  >
                    <LinuxDoMark class="h-5 w-5" />
                  </span>
                  <span
                    class="flex h-10 w-10 items-center justify-center rounded-control bg-gray-950/[0.03] text-primary-600 dark:bg-dark-50/[0.03] dark:text-primary-500"
                  >
                    <svg
                      class="h-5 w-5"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="1.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                    >
                      <path :d="onboardingPaths.envelope" />
                    </svg>
                  </span>
                </div>

                <div v-else-if="step.key === 'recharge'" class="flex flex-col gap-2">
                  <div
                    v-for="example in homeRechargeExamples"
                    :key="example.date"
                    class="home-step-sample flex h-6 items-center gap-2 bg-gray-950/5 px-2 dark:bg-dark-50/5"
                  >
                    <span
                      class="shrink-0 text-xs leading-[1.35] text-gray-950/[0.69] dark:text-dark-50/[0.69]"
                    >
                      {{
                        formatDate(
                          example.date,
                          { month: 'short', day: 'numeric', timeZone: 'UTC' },
                          locale,
                        )
                      }}
                    </span>
                    <span
                      class="home-step-sample h-2 min-w-0 flex-1 bg-gray-950/[0.08] dark:bg-dark-50/[0.08]"
                    ></span>
                    <span
                      class="home-step-sample h-2 min-w-0 flex-1 bg-gray-950/[0.08] dark:bg-dark-50/[0.08]"
                    ></span>
                    <span class="shrink-0 font-medium">{{ example.amount }}</span>
                  </div>
                </div>

                <div
                  v-else
                  class="home-step-sample flex h-6 items-center bg-gray-950/5 px-2 text-xs leading-[1.35] tracking-wider text-gray-950/[0.69] dark:bg-dark-50/5 dark:text-dark-50/[0.69]"
                >
                  ••••••••••••••••
                </div>
              </div>
            </article>
          </div>
        </section>

        <!-- The closing signal band reserves its dark surface while its renderer waits near the viewport. -->
        <section class="mx-auto mt-20 max-w-7xl md:mt-24">
          <div
            class="relative isolate min-h-[340px] overflow-hidden rounded-surface border border-black/[0.10] bg-[#141413] shadow-xl sm:min-h-[380px] dark:border-white/[0.12]"
          >
            <div class="absolute inset-0 z-0">
              <LazyVisual class="absolute inset-0">
                <MicroSlats
                  preset="swell"
                  :color="slatsTheme.color"
                  :glint-color="slatsTheme.glint"
                  :background-color="slatsTheme.background"
                  :slat-width="10"
                  :slat-height="25"
                  :gap="3"
                  :roundness="0.75"
                  interactive
                  :cursor-strength="1.15"
                  :cursor-size="44"
                  :swirl="0.35"
                  :trail="1.6"
                  :lean="0.35"
                  intro
                  :max-dpr="1"
                  :max-fps="30"
                />
              </LazyVisual>
              <div
                class="pointer-events-none absolute inset-0 bg-gradient-to-r from-[#141413]/85 via-[#141413]/45 to-transparent"
              ></div>
            </div>

            <div
              class="pointer-events-none relative z-10 flex min-h-[340px] flex-col justify-center p-7 sm:min-h-[380px] sm:p-10 lg:p-12"
            >
              <div class="max-w-xl py-4">
                <h2
                  class="text-balance text-3xl font-light leading-[1.12] tracking-[-0.03em] text-white sm:text-4xl"
                >
                  {{ homeHeroTitle }}
                </h2>
                <p
                  class="mt-3 max-w-[46ch] text-pretty text-sm leading-relaxed text-white/75 sm:text-base"
                >
                  {{ homeHeroSubtitle }}
                </p>
                <div class="pointer-events-auto mt-7 flex flex-wrap items-center gap-3">
                  <router-link
                    :to="isAuthenticated ? dashboardPath : '/login'"
                    class="inline-flex min-h-11 items-center justify-center gap-2 rounded-full bg-white px-6 py-2.5 text-sm font-medium text-gray-950 shadow-sm transition duration-fast ease-standard hover:-translate-y-0.5 hover:bg-gray-100 active:scale-[0.98]"
                  >
                    {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                    <Icon name="arrowRight" size="sm" :stroke-width="2" />
                  </router-link>
                  <router-link
                    to="/models"
                    class="inline-flex min-h-11 items-center justify-center gap-2 rounded-full border border-white/25 bg-black/45 px-6 py-2.5 text-sm font-medium text-white transition duration-fast ease-standard hover:-translate-y-0.5 hover:bg-black/65 active:scale-[0.98]"
                  >
                    {{ t('home.exploreMarketplace') }}
                  </router-link>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </main>

    <!-- Site navigation and published policies share the page backdrop; configured groups replace the default destinations. -->
    <footer
      class="relative z-10 border-t border-black/[0.07] px-6 pb-8 pt-12 dark:border-dark-600 md:px-12 md:pt-16"
    >
      <div class="mx-auto max-w-7xl">
        <div class="grid gap-10 lg:grid-cols-12 lg:gap-16">
          <div class="min-w-0 lg:col-span-4">
            <router-link
              to="/home"
              class="inline-flex max-w-full items-center gap-2.5 rounded-compact focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-primary-500"
            >
              <span class="h-8 w-8 shrink-0 overflow-hidden rounded-control shadow-sm">
                <img :src="siteLogo || '/logo.svg'" alt="" class="h-full w-full object-contain" />
              </span>
              <span
                class="min-w-0 break-words text-base font-semibold text-gray-950 dark:text-white"
              >
                {{ siteName }}
              </span>
            </router-link>
            <p class="mt-4 max-w-sm text-sm leading-relaxed text-gray-500 dark:text-dark-400">
              {{ t('home.footer.description') }}
            </p>
            <p
              v-if="appStore.cachedPublicSettings?.google_oauth_enabled === true"
              class="mt-4 max-w-sm text-sm leading-relaxed text-gray-500 dark:text-dark-400"
            >
              {{ t('home.footer.googleSignIn') }}
            </p>
            <p class="mt-4 max-w-sm text-sm leading-relaxed text-gray-500 dark:text-dark-400">
              {{ t('home.footer.acceptableUse') }}
            </p>
          </div>

          <nav
            :aria-label="t('home.footer.navigation')"
            class="grid min-w-0 grid-cols-2 gap-x-8 gap-y-8 sm:grid-cols-3 lg:col-span-8"
          >
            <div v-for="column in footerColumns" :key="column.title" class="min-w-0">
              <h2 class="break-words text-sm font-semibold text-gray-950 dark:text-white">
                {{ column.title }}
              </h2>
              <ul class="mt-3 space-y-1">
                <li v-for="link in column.links" :key="link.url">
                  <router-link
                    v-if="link.url.startsWith('/')"
                    :to="link.url"
                    class="inline-block min-h-8 max-w-full break-words rounded-compact py-1 text-sm leading-relaxed text-gray-500 transition-colors duration-fast hover:text-gray-950 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-primary-500 dark:text-dark-400 dark:hover:text-white"
                  >
                    {{ link.label }}
                  </router-link>
                  <a
                    v-else
                    :href="link.url"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="inline-block min-h-8 max-w-full break-words rounded-compact py-1 text-sm leading-relaxed text-gray-500 transition-colors duration-fast hover:text-gray-950 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-primary-500 dark:text-dark-400 dark:hover:text-white"
                  >
                    {{ link.label }}
                  </a>
                </li>
              </ul>
            </div>
          </nav>
        </div>

        <div class="mt-10 border-t border-black/[0.07] pt-6 dark:border-dark-600">
          <p class="break-words text-xs leading-relaxed text-gray-500 dark:text-dark-400">
            &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
          </p>
          <p
            v-for="(line, index) in footerTextLines"
            :key="index"
            class="mt-1 break-words text-xs leading-relaxed text-gray-500 dark:text-dark-400"
          >
            {{ line }}
          </p>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { useLocaleRefresh } from '@/composables/useLocaleRefresh'
import { vContentReveal } from '@/directives/contentReveal'
import { useRoute as useMotionRoute } from 'vue-router'

import MotionTransition from '@/components/common/MotionTransition.vue'
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import GitHubMark from '@/components/auth/GitHubMark.vue'
import GoogleMark from '@/components/common/GoogleMark.vue'
import GoogleOneTap from '@/components/auth/GoogleOneTap.vue'
import LinuxDoMark from '@/components/auth/LinuxDoMark.vue'
import AppHeader from '@/components/layout/AppHeader.vue'
import GradientASCII from '@/components/common/GradientASCII.vue'
import LazyVisual from '@/components/common/LazyVisual.vue'
import heroDitherArt from '@/assets/hero-dither-art.jpg'

// Below-the-fold modules are requested only when their LazyVisual boundary activates;
// the initial hero and page content do not wait for OGL or Matter.js.
const DitherVeil = defineAsyncComponent(() => import('@/components/common/DitherVeil.vue'))
const FallingText = defineAsyncComponent(() => import('@/components/common/FallingText.vue'))
const MicroSlats = defineAsyncComponent(() => import('@/components/common/MicroSlats.vue'))
import { useTheme } from '@/composables/useTheme'
import ProviderIcon from '@/components/common/ProviderIcon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import ModelCapabilityTags from '@/components/marketplace/ModelCapabilityTags.vue'
import Skeleton from '@/components/common/Skeleton.vue'
import Icon from '@/components/icons/Icon.vue'
import { getMarketplaceModels, getMarketplaceStats } from '@/api/marketplace'
import type { MarketplaceGroup, MarketplaceModel, MarketplaceStats } from '@/api/marketplace.types'
import type { FooterLinkGroup } from '@/api/settings.types'
import { formatDate } from '@/utils/format'
import { onboardingPaths } from './home/onboardingArtwork'
import { modelAuthor } from './home/modelAuthor'
import { sanitizeUrl } from '@/utils/url'
import { hasAcceptedLoginAgreement } from '@/utils/loginAgreement'
import { isGoogleOneTapEligible, isGoogleOneTapOriginSupported } from '@/utils/googleIdentity'
import {
  providerBrandDisplayName,
  providerBrandFilterKey,
  resolveProviderBrand,
  resolveProviderBrandKey,
} from '@/utils/providerBrand'

// HomeView.vue combines public branding, optional custom home content, and the
// built-in marketplace landing page. Configured HTTP(S) content uses an iframe,
// while configured HTML is inserted directly. The default page groups public
// models by provider brand, resolves configured featured aliases, and displays
// independently loaded usage totals with one initial animation per statistic.
//
// Page setup retains shared account and public-settings initialization. The
// initial marketplace and statistics reads run together only for the default
// page; locale changes separately refresh marketplace records. A tracked interval
// rotates the marketplace action's brand icon, and tracked animation frames update
// statistics. Teardown clears those handles while in-flight API requests retain
// their existing independent completion paths without cancellation or identity guards.

// motionRoute supplies the path used to replay the default page's reveal animation.
const motionRoute = useMotionRoute()

// Theme state supplies the active palette and light mode to the homepage artwork.
const { isDark: isDarkTheme, themePalette } = useTheme()

// heroVeilTheme follows the active theme and palette to color the fixed Floyd dither artwork.
const heroVeilTheme = computed(() => {
  const palette = themePalette.value
  if (isDarkTheme.value) {
    if (palette === 'indigo') return { ink: '#0C0F17', paper: '#C8D0E4', rim: '#8B9AD1' }
    if (palette === 'pearl') return { ink: '#0E0E0D', paper: '#D4D4D0', rim: '#E7E5DF' }
    return { ink: '#0E0E0D', paper: '#D6CEC2', rim: '#E38A6E' }
  }
  if (palette === 'indigo') return { ink: '#171B26', paper: '#F1F3F8', rim: '#4E5D94' }
  if (palette === 'pearl') return { ink: '#191918', paper: '#F4F4F2', rim: '#6B6A66' }
  return { ink: '#1E1A17', paper: '#F5F2EC', rim: '#C46346' }
})

// heroAsciiColors follows the active theme and palette to color the GradientASCII backdrop.
const heroAsciiColors = computed<string[]>(() => {
  const palette = themePalette.value
  if (isDarkTheme.value) {
    if (palette === 'indigo') return ['#1e3fd8', '#38bdf8', '#a5d8ff', '#f6f8ff']
    if (palette === 'pearl') return ['#2E2D2B', '#6B6963', '#B8B4AC', '#FAF9F6']
    return ['#7C2D12', '#C46346', '#E59A78', '#FDE6D8']
  }
  if (palette === 'indigo') return ['#1e3fd8', '#3b82f6', '#93c5fd', '#eff6ff']
  if (palette === 'pearl') return ['#18181B', '#52525B', '#A1A1AA', '#F4F4F5']
  return ['#9A3412', '#C46346', '#E08E6D', '#F7D9C8']
})

// slatsTheme follows the active palette to color the closing MicroSlats fluid signal band.
const slatsTheme = computed(() => {
  const palette = themePalette.value
  if (palette === 'indigo') {
    return { color: '#7170ff', glint: '#eef2ff', background: '#0d1018' }
  }
  if (palette === 'pearl') {
    return { color: '#d4d2c9', glint: '#faf9f5', background: '#141413' }
  }
  return { color: '#d97757', glint: '#faf9f5', background: '#141413' }
})

// fallingAccentColor follows the active theme and palette to highlight model names
// in the physics band.
const fallingAccentColor = computed(() => {
  const palette = themePalette.value
  if (palette === 'indigo') return isDarkTheme.value ? '#7170ff' : '#5e6ad2'
  if (palette === 'pearl') return isDarkTheme.value ? '#e8e6dc' : '#30302e'
  return isDarkTheme.value ? '#d97757' : '#c96442'
})

// HomeStatsKey identifies one independently animated statistic in the public landing page.
type HomeStatsKey = 'today-tokens' | 'total-tokens' | 'total-users' | 'supported-models'
// HomeStatsIcon restricts configured statistic icon metadata to the supported icon names.
type HomeStatsIcon = 'bolt' | 'database' | 'users' | 'grid'
// HomeStepIcon identifies the configured icon for a public onboarding step.
type HomeStepIcon = 'user' | 'creditCard' | 'key'
// HomeStatFormat chooses compact notation or whole-number formatting for animated totals.
type HomeStatFormat = 'compact' | 'number'

// HomeProviderCategory groups marketplace records under one displayed provider identity.
interface HomeProviderCategory {
  key: string // Normalized category identity.
  label: string // Displayed provider category.
  iconBrand: string // Shared artwork brand identifier.
}

// HomeProviderSummary aggregates model entries and display ordering for a provider category.
interface HomeProviderSummary extends HomeProviderCategory {
  modelCount: number // Model entries across contributing groups.
  groupCount: number // Contributing group record count.
  // officialPriceRatio is the smallest valid positive group ratio; omission means
  // no usable official-price comparison was reported for this provider category.
  officialPriceRatio?: number
  sortOrder: number // Minimum configured group order.
  firstIndex: number // First sorted group position.
}

// HomeStatsCard carries the configured label, formatted value, and statistic icon metadata.
interface HomeStatsCard {
  key: HomeStatsKey // Animated statistic identity.
  label: string // Translated statistic caption.
  // value contains formatted statistic text, with loading or missing placeholders
  // displayed until the corresponding animated numeric value becomes available.
  value: string
  icon: HomeStatsIcon // Configured statistic icon identifier.
  iconWrapClass: string // Configured icon wrapper classes.
  iconClass: string // Configured icon foreground classes.
}

// HomeStep describes one numbered instruction in the landing page's introductory sequence.
interface HomeStep {
  key: string // Stable instruction identity.
  index: number // Displayed one-based step number.
  title: string // Translated instruction heading.
  description: string // Translated instruction description.
  icon: HomeStepIcon // Configured instruction icon name.
  illustrationLineWidths: string[] // Placeholder bar width classes.
}

// HomeFeaturedModel pairs a public model with its developer byline and formatted discount.
interface HomeFeaturedModel {
  model: MarketplaceModel // Selected public model record.
  // author names the developer of the recognized model family. Null omits the
  // byline because the marketplace group only identifies a serving product.
  author: string | null
  // discountOff displays savings against the official reference price for the group.
  // A missing, invalid, or undiscounted ratio leaves the value unavailable.
  discountOff: string | null
}

// Shared services provide language, account role, and cached public settings for
// the page's branding, navigation destinations, and optional authentication prompt.
const { t, locale } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Branding reads injected or cached public settings, filtering logo and documentation
// URLs while retaining explicitly configured title and subtitle overrides, including empty text.
const siteName = computed(() => appStore.siteName || 'TokenRouter')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true,
  }),
)
const docUrl = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''),
)
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const numberLocale = computed(() => locale.value)
const homeHeroTitle = computed(() =>
  appStore.cachedPublicSettings?.site_text_overrides?.includes('site_title')
    ? appStore.cachedPublicSettings.site_title || ''
    : t('home.heroTitle'),
)
const homeHeroSubtitle = computed(() =>
  appStore.cachedPublicSettings?.site_text_overrides?.includes('site_subtitle')
    ? appStore.cachedPublicSettings.site_subtitle || ''
    : t('home.heroDescription'),
)

// Custom content uses the existing HTTP(S)-prefix decision to choose an iframe;
// every other nonempty value follows the administrator-configured HTML rendering path.
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Authentication and role select the primary navigation destination without
// changing the independently evaluated eligibility for the Google One Tap prompt.
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))

// One Tap eligibility combines loaded public flags, account state, consent for
// the current agreement revision, captcha policy, client identity, and origin support.
const googleOneTapClientID = computed(
  () => appStore.cachedPublicSettings?.google_oauth_client_id || '',
)
const googleOneTapEligible = computed(() => {
  const settings = appStore.cachedPublicSettings
  if (!settings) return false
  const agreementEnabled = settings.login_agreement_enabled === true
  return isGoogleOneTapEligible({
    publicSettingsLoaded: appStore.publicSettingsLoaded,
    isAuthenticated: isAuthenticated.value,
    oneTapEnabled: settings.google_one_tap_enabled === true,
    clientID: googleOneTapClientID.value,
    backendModeEnabled: settings.backend_mode_enabled,
    tencentCaptchaEnabled: settings.tencent_captcha_enabled === true,
    aliyunCaptchaEnabled: settings.aliyun_captcha_enabled === true,
    loginAgreementEnabled: agreementEnabled,
    loginAgreementAccepted:
      !agreementEnabled || hasAcceptedLoginAgreement(settings.login_agreement_revision || ''),
    originSupported: isGoogleOneTapOriginSupported(),
  })
})

// Copyright text uses a lazily evaluated year snapshot; no clock dependency
// forces it to recompute while this page remains mounted across a year boundary.
const currentYear = computed(() => new Date().getFullYear())

// Footer text keeps administrator-provided lines below the copyright, excluding
// blank lines without changing their order or other visible text.
const footerTextLines = computed<string[]>(() => {
  const raw = appStore.cachedPublicSettings?.footer_text || ''
  return raw
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
})

// footerColumns prefers configured groups; otherwise public routes, account state,
// and published legal documents provide the default navigation without invented destinations.
const footerColumns = computed<FooterLinkGroup[]>(() => {
  const configured = (appStore.cachedPublicSettings?.footer_links || [])
    .filter((group) => group.title && Array.isArray(group.links) && group.links.length > 0)
    .map((group) => ({
      title: group.title,
      links: group.links.filter((link) => link.label && link.url),
    }))
    .filter((group) => group.links.length > 0)

  // A usable configured group replaces all built-in columns; incomplete
  // groups and links are filtered before this decision uses their remaining count.
  if (configured.length > 0) {
    return configured
  }

  const productLinks = [
    { label: t('home.footer.modelsAndPricing'), url: '/models' },
    { label: t('keyUsage.title'), url: '/key-usage' },
  ]
  if (docUrl.value) {
    productLinks.push({ label: t('home.docs'), url: docUrl.value })
  }

  const accountLinks = isAuthenticated.value
    ? [
        { label: t('home.dashboard'), url: dashboardPath.value },
        { label: t('home.footer.apiKeys'), url: '/keys' },
      ]
    : [{ label: t('home.login'), url: '/login' }]
  if (!isAuthenticated.value && appStore.cachedPublicSettings?.registration_enabled !== false) {
    accountLinks.push({ label: t('auth.createAccount'), url: '/register' })
  }

  const columns: FooterLinkGroup[] = [
    { title: t('home.footer.product'), links: productLinks },
    { title: t('home.footer.account'), links: accountLinks },
  ]
  // Missing summaries in older startup settings use the server's built-in document
  // IDs. An explicitly published list controls which policies are linked,
  // including custom privacy documents.
  const legalDocuments = appStore.cachedPublicSettings?.login_agreement_documents ?? [
    { id: 'terms', title: t('home.footer.terms') },
    { id: 'usage-policy', title: t('home.footer.usagePolicy') },
    { id: 'supported-regions', title: t('home.footer.supportedRegions') },
    { id: 'service-specific-terms', title: t('home.footer.serviceTerms') },
  ]
  const legalLinks = legalDocuments
    .filter((document) => document.id?.trim() && document.title?.trim())
    .map((document) => ({
      label: document.title,
      url: `/legal/${encodeURIComponent(document.id)}`,
    }))
  if (legalLinks.length > 0) {
    columns.push({ title: t('home.footer.legal'), links: legalLinks })
  }
  return columns
})

// Marketplace and aggregate statistics keep independent loading/error states so
// one failed read can clear its content without discarding the other's result.
const marketplaceGroups = ref<MarketplaceGroup[]>([])
const homeStats = ref<MarketplaceStats | null>(null)
const homeMarketplaceLoading = ref(true)
const homeStatsLoading = ref(true)
const homeMarketplaceError = ref(false)
const homeStatsError = ref(false)
// The marketplace action cycles through loaded provider brands using one tracked
// interval and a bounded display index; empty lists use the template fallback icon.
const homeMarketplaceButtonIconIndex = ref(0)
let homeMarketplaceButtonIconTimer: number | null = null
// Statistic values, started keys, and active frame handles coordinate one initial
// count-up per key; teardown cancels frames still present in the handle map.
const homeAnimatedStats = ref<Record<HomeStatsKey, number>>({
  'today-tokens': 0,
  'total-tokens': 0,
  'total-users': 0,
  'supported-models': 0,
})
const homeAnimatedStatKeys = new Set<HomeStatsKey>()
const homeStatAnimationFrames = new Map<HomeStatsKey, number>()
const homeStatAnimationDurationMs = 3200

// homeRouteLabel identifies the example model shown above the DitherVeil artwork
// on the public landing page.
const homeRouteLabel = 'GPT-6 Astra'

// totalModelCount counts model entries across groups, including aliases repeated
// in separate groups; it supplies both the summary and supported-model animation target.
const totalModelCount = computed(() =>
  marketplaceGroups.value.reduce((total, group) => total + group.models.length, 0),
)

// homeFeaturedModelIds reads the configured display order from public settings;
// an absent or non-array value leaves the page using provider-category cards.
const homeFeaturedModelIds = computed<string[]>(() => {
  const configured = appStore.cachedPublicSettings?.home_featured_models
  return Array.isArray(configured) ? configured : []
})

// featuredModels resolves configured aliases in order and takes the first matching
// marketplace group for each alias. Unresolved identifiers contribute no featured card.
const featuredModels = computed<HomeFeaturedModel[]>(() => {
  const resolved: HomeFeaturedModel[] = []
  for (const modelId of homeFeaturedModelIds.value) {
    for (const group of marketplaceGroups.value) {
      const model = group.models.find((item) => item.id === modelId)
      if (model) {
        resolved.push({
          model,
          author: modelAuthor(model.id),
          discountOff: formatFeaturedDiscountOff(group.official_price_ratio),
        })
        break
      }
    }
  }
  return resolved
})

// Animation targets remain null during their owning load and normalize invalid
// values afterward; the watcher starts each valid key only once per mounted page.
const homeStatAnimationTargets = computed<Record<HomeStatsKey, number | null>>(() => ({
  'today-tokens': homeStatsLoading.value
    ? null
    : normalizedHomeStatTarget(homeStats.value?.today_tokens),
  'total-tokens': homeStatsLoading.value
    ? null
    : normalizedHomeStatTarget(homeStats.value?.total_tokens),
  'total-users': homeStatsLoading.value
    ? null
    : normalizedHomeStatTarget(homeStats.value?.total_users),
  'supported-models': homeMarketplaceLoading.value
    ? null
    : normalizedHomeStatTarget(totalModelCount.value),
}))

// supportedProviders groups nonempty marketplace records by resolved brand category.
// It preserves the smallest positive official ratio and earliest configured position,
// then applies category priority before group ordering for the resulting public cards.
const supportedProviders = computed<HomeProviderSummary[]>(() => {
  const summaries = new Map<string, HomeProviderSummary>()
  const sortedGroups = [...marketplaceGroups.value].sort((left, right) => {
    const sortDiff = (left.sort_order ?? 0) - (right.sort_order ?? 0)
    if (sortDiff !== 0) {
      return sortDiff
    }
    return left.id - right.id
  })

  sortedGroups.forEach((group, index) => {
    const modelCount = group.models.length
    if (modelCount === 0) {
      return
    }

    const category = homeProviderCategory(group)
    const existing = summaries.get(category.key)
    const ratio = validOfficialPriceRatio(group.official_price_ratio)
    if (!existing) {
      summaries.set(category.key, {
        ...category,
        modelCount,
        groupCount: 1,
        officialPriceRatio: ratio ?? undefined,
        sortOrder: group.sort_order ?? 0,
        firstIndex: index,
      })
      return
    }

    existing.modelCount += modelCount
    existing.groupCount += 1
    existing.sortOrder = Math.min(existing.sortOrder, group.sort_order ?? 0)
    existing.firstIndex = Math.min(existing.firstIndex, index)
    if (ratio && (!existing.officialPriceRatio || ratio < existing.officialPriceRatio)) {
      existing.officialPriceRatio = ratio
    }
  })

  return [...summaries.values()].sort((left, right) => {
    const priorityDiff = homeProviderPriority(left.key) - homeProviderPriority(right.key)
    if (priorityDiff !== 0) {
      return priorityDiff
    }
    const sortDiff = left.sortOrder - right.sortOrder
    if (sortDiff !== 0) {
      return sortDiff
    }
    return left.firstIndex - right.firstIndex
  })
})

// Statistic cards combine translated captions and animated values with their
// existing icon metadata, using whole-number formatting for users and supported model entries.
const homeStatsCards = computed<HomeStatsCard[]>(() => [
  {
    key: 'today-tokens',
    label: t('home.stats.todayTokens'),
    value: formatAnimatedHomeStat(
      'today-tokens',
      homeStatAnimationTargets.value['today-tokens'],
      homeStatsLoading.value,
    ),
    icon: 'bolt',
    iconWrapClass: 'bg-sky-100 dark:bg-sky-500/15',
    iconClass: 'text-sky-600 dark:text-sky-300',
  },
  {
    key: 'total-tokens',
    label: t('home.stats.totalTokens'),
    value: formatAnimatedHomeStat(
      'total-tokens',
      homeStatAnimationTargets.value['total-tokens'],
      homeStatsLoading.value,
    ),
    icon: 'database',
    iconWrapClass: 'bg-emerald-100 dark:bg-emerald-500/15',
    iconClass: 'text-emerald-600 dark:text-emerald-300',
  },
  {
    key: 'total-users',
    label: t('home.stats.totalUsers'),
    value: formatAnimatedHomeStat(
      'total-users',
      homeStatAnimationTargets.value['total-users'],
      homeStatsLoading.value,
      'number',
    ),
    icon: 'users',
    iconWrapClass: 'bg-violet-100 dark:bg-violet-500/15',
    iconClass: 'text-violet-600 dark:text-violet-300',
  },
  {
    key: 'supported-models',
    label: t('home.stats.supportedModels'),
    value: formatAnimatedHomeStat(
      'supported-models',
      homeStatAnimationTargets.value['supported-models'],
      homeMarketplaceLoading.value,
      'number',
    ),
    icon: 'grid',
    iconWrapClass: 'bg-primary-100 dark:bg-primary-500/15',
    iconClass: 'text-primary-600 dark:text-primary-300',
  },
])

// The marketplace action rotates through the provider categories present in the catalog.
const homeMarketplaceButtonBrands = computed(() =>
  supportedProviders.value.map((provider) => provider.iconBrand),
)

const homeMarketplaceButtonBrand = computed(() => {
  const brands = homeMarketplaceButtonBrands.value
  if (brands.length === 0) {
    return ''
  }
  return brands[homeMarketplaceButtonIconIndex.value % brands.length]
})

// homeSteps translates the signup, balance top-up, and API key instructions in
// order and pairs each step with the icon used in its illustration.
const homeSteps = computed<HomeStep[]>(() => [
  {
    key: 'signup',
    index: 1,
    title: t('home.steps.signup.title'),
    description: t('home.steps.signup.description'),
    icon: 'user',
    illustrationLineWidths: ['w-6', 'w-12'],
  },
  {
    key: 'recharge',
    index: 2,
    title: t('home.steps.recharge.title'),
    description: t('home.steps.recharge.description'),
    icon: 'creditCard',
    illustrationLineWidths: ['w-6', 'w-6', 'w-6', 'w-6'],
  },
  {
    key: 'api-key',
    index: 3,
    title: t('home.steps.apiKey.title'),
    description: t('home.steps.apiKey.description'),
    icon: 'key',
    illustrationLineWidths: [],
  },
])

// homeRechargeExamples supplies fixed dates and amounts for the homepage recharge
// illustration. These examples never read the current user's payment history.
const homeRechargeExamples = [
  { date: '2025-04-01', amount: '$99' },
  { date: '2025-03-30', amount: '$10' },
]

// homeProviderCategory chooses an explicit display brand before the trimmed group name.
// Known brands use the shared category mapping. Unknown brands retain their selected
// label and derive a filter key without discarding the group's visible identity.
function homeProviderCategory(group: MarketplaceGroup): HomeProviderCategory {
  const brandSource = group.display_brand?.trim() || group.name.trim()
  const brandKey = resolveProviderBrandKey(brandSource)
  if (brandKey && brandKey !== 'unknown') {
    return homeProviderCategoryFromBrand(brandKey, brandSource)
  }

  const fallbackLabel = brandSource
  return {
    key: providerBrandFilterKey(fallbackLabel),
    label: fallbackLabel,
    iconBrand: fallbackLabel,
  }
}

// homeProviderCategoryFromBrand maps core provider brands to localized home categories.
// Other recognized brands retain the shared display name and brand key, allowing
// the same aggregation path to cover providers outside the three core categories.
function homeProviderCategoryFromBrand(brandKey: string, source: string): HomeProviderCategory {
  switch (brandKey) {
    case 'anthropic':
      return { key: 'claude', label: t('home.providers.claude'), iconBrand: 'Claude' }
    case 'openai':
      return { key: 'gpt', label: t('home.providers.gpt'), iconBrand: 'OpenAI' }
    case 'google':
      return { key: 'gemini', label: t('home.providers.gemini'), iconBrand: 'Gemini' }
    default: {
      const label = providerBrandDisplayName(source)
      return { key: brandKey || providerBrandFilterKey(source), label, iconBrand: label }
    }
  }
}

// homeProviderPriority ranks preferred categories before all remaining provider identities.
function homeProviderPriority(key: string): number {
  const priorities = ['claude', 'gpt', 'deepseek', 'gemini', 'antigravity']
  const index = priorities.indexOf(key)
  return index === -1 ? priorities.length : index
}

// validOfficialPriceRatio returns a finite positive ratio for official-price comparisons.
// Missing, nonnumeric, nonfinite, and nonpositive values return null, allowing
// provider summaries and featured cards to omit unusable comparisons.
function validOfficialPriceRatio(value?: number): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null
}

// formatOfficialPriceRatio translates the ratio expressed on the existing ten-point scale.
function formatOfficialPriceRatio(ratio: number): string {
  const discount = new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  }).format(ratio * 10)

  return t('marketplace.officialPriceDiscount', { discount })
}

// formatFeaturedDiscountOff formats savings only for a positive ratio below one.
// Invalid or undiscounted comparisons return null so the card can show a placeholder.
// Percentage formatting uses the active interface locale and one fractional digit.
function formatFeaturedDiscountOff(ratio?: number): string | null {
  const valid = validOfficialPriceRatio(ratio)
  if (valid === null || valid >= 1) {
    return null
  }
  return new Intl.NumberFormat(numberLocale.value, {
    style: 'percent',
    maximumFractionDigits: 1,
  }).format(1 - valid)
}

// formatAnimatedHomeStat formats the current animated value for a prepared target.
// Loading and invalid targets produce distinct placeholders. Available targets use
// compact or whole-number output and retain the existing trailing plus marker.
function formatAnimatedHomeStat(
  key: HomeStatsKey,
  target: number | null,
  loading: boolean,
  format: HomeStatFormat = 'compact',
): string {
  if (loading) {
    return '...'
  }
  if (target === null) {
    return '-'
  }

  const value = homeAnimatedStats.value[key]
  const formatted =
    format === 'compact' ? formatAnimatedCompactNumber(value, target) : formatWholeNumber(value)
  return `${formatted}+`
}

// formatMarketplaceStat formats a marketplace count using the active number locale.
// While the marketplace request is loading, it returns the three-dot placeholder
// regardless of the supplied count.
function formatMarketplaceStat(value: number): string {
  if (homeMarketplaceLoading.value) {
    return '...'
  }
  return new Intl.NumberFormat(numberLocale.value).format(value)
}

// formatAnimatedCompactNumber formats an animation value using its target's compact
// suffix and precision. The caller supplies the final target alongside the current
// frame value so crossing a compact-number threshold does not change the suffix
// during the count-up. A target containing a compact suffix delegates scaling to
// scaleCompactValue; otherwise the frame value stays unscaled. Fractional digits
// appear only when that compact target has a fractional part, keeping the displayed
// precision consistent throughout the animation.
function formatAnimatedCompactNumber(value: number, target: number): string {
  const targetParts = new Intl.NumberFormat(numberLocale.value, {
    notation: 'compact',
    maximumFractionDigits: 1,
  }).formatToParts(target)
  const compactPart = targetParts.find((part) => part.type === 'compact')?.value ?? ''
  const scaledValue = compactPart ? scaleCompactValue(value, target) : value
  const decimalDigits = compactPart && targetParts.some((part) => part.type === 'fraction') ? 1 : 0
  const numberText = new Intl.NumberFormat(numberLocale.value, {
    minimumFractionDigits: decimalDigits,
    maximumFractionDigits: decimalDigits,
    useGrouping: false,
  }).format(scaledValue)

  return `${numberText}${compactPart}`
}

// scaleCompactValue derives a scale from the target's compact numeric parts.
// The existing conversion uses Number on concatenated locale parts. Nonfinite or
// nonpositive parsed targets return the original value instead of applying a scale.
function scaleCompactValue(value: number, target: number): number {
  const compactTargetNumber = Number(
    new Intl.NumberFormat(numberLocale.value, {
      notation: 'compact',
      maximumFractionDigits: 1,
    })
      .formatToParts(target)
      .filter(
        (part) => part.type === 'integer' || part.type === 'decimal' || part.type === 'fraction',
      )
      .map((part) => part.value)
      .join(''),
  )
  if (!Number.isFinite(compactTargetNumber) || compactTargetNumber <= 0) {
    return value
  }

  return value / (target / compactTargetNumber)
}

// formatWholeNumber rounds the animated value and formats it without grouping separators.
function formatWholeNumber(value: number): string {
  return new Intl.NumberFormat(numberLocale.value, {
    maximumFractionDigits: 0,
    useGrouping: false,
  }).format(Math.round(value))
}

// normalizedHomeStatTarget converts finite values into nonnegative animation targets.
// Negative numbers become zero; missing, nonnumeric, and nonfinite values return
// null so their counters retain the existing unavailable-value display.
function normalizedHomeStatTarget(value?: number): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, value) : null
}

// startHomeStatAnimation starts or replaces the tracked count-up for one statistic.
// The key is marked started before any frame is scheduled, and a previous frame
// for that key is cancelled. Reduced-motion preferences and a zero target publish
// the final value immediately without creating another animation-frame handle.
//
// Otherwise each frame applies the configured easing from zero toward the captured
// target and replaces its own map entry while more frames are needed. Completion
// publishes the exact target and removes that entry. The target watcher admits
// only keys that have not started, and component teardown cancels the remaining
// registered handles without resetting the started-key set.
function startHomeStatAnimation(key: HomeStatsKey, target: number) {
  homeAnimatedStatKeys.add(key)
  if (homeStatAnimationFrames.has(key)) {
    cancelAnimationFrame(homeStatAnimationFrames.get(key)!)
    homeStatAnimationFrames.delete(key)
  }

  const reduceMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
  if (reduceMotion || target === 0) {
    homeAnimatedStats.value = { ...homeAnimatedStats.value, [key]: target }
    return
  }

  const startTime = performance.now()
  // tick updates one statistic against the captured start time and target.
  // It records the next frame handle until progress reaches one, then publishes
  // the exact target and removes its handle from teardown ownership.
  const tick = (now: number) => {
    const progress = Math.min((now - startTime) / homeStatAnimationDurationMs, 1)
    // Quartic easing advances quickly at first and slows near the target,
    // while the shared duration keeps all admitted statistics on the same timing scale.
    const easedProgress = 1 - Math.pow(1 - progress, 4)
    homeAnimatedStats.value = {
      ...homeAnimatedStats.value,
      [key]: target * easedProgress,
    }

    if (progress < 1) {
      homeStatAnimationFrames.set(key, requestAnimationFrame(tick))
      return
    }

    homeAnimatedStats.value = { ...homeAnimatedStats.value, [key]: target }
    homeStatAnimationFrames.delete(key)
  }

  homeStatAnimationFrames.set(key, requestAnimationFrame(tick))
}

// Admit each statistic once when its first valid target arrives; later target
// changes retain the existing started-key behavior instead of restarting its count-up.
watch(
  homeStatAnimationTargets,
  (targets) => {
    const statEntries = Object.entries(targets) as Array<[HomeStatsKey, number | null]>
    statEntries.forEach(([key, target]) => {
      if (target === null || homeAnimatedStatKeys.has(key)) {
        return
      }
      startHomeStatAnimation(key, target)
    })
  },
  { immediate: true },
)

// Keep the rotating action index valid as available brands change, resetting it
// when no catalog brands remain and otherwise reducing it to the current list length.
watch(
  homeMarketplaceButtonBrands,
  (brands) => {
    if (brands.length === 0) {
      homeMarketplaceButtonIconIndex.value = 0
      return
    }
    homeMarketplaceButtonIconIndex.value %= brands.length
  },
  { immediate: true },
)

// providerIconWrapClass applies the home-specific Antigravity palette before shared brand styling.
function providerIconWrapClass(provider: Pick<HomeProviderSummary, 'key' | 'iconBrand'>): string {
  if (provider.key === 'antigravity') {
    return 'bg-rose-50 text-rose-700 ring-rose-200 dark:bg-rose-500/15 dark:text-rose-200 dark:ring-rose-400/30'
  }
  return resolveProviderBrand(provider.iconBrand).iconWrapClass
}

// fetchHomeMarketplace replaces public groups and reports its own loading outcome.
// Failure clears the previous groups and exposes the unavailable state. Concurrent
// calls retain completion-order publication without a generation guard or cancellation signal.
async function fetchHomeMarketplace() {
  homeMarketplaceLoading.value = true
  homeMarketplaceError.value = false

  try {
    marketplaceGroups.value = await getMarketplaceModels()
  } catch (error) {
    // Clear stale groups and mark this source unavailable so provider cards and
    // model totals reflect the failed refresh while independently loaded statistics remain.
    console.error('Failed to load home marketplace models:', error)
    marketplaceGroups.value = []
    homeMarketplaceError.value = true
  } finally {
    homeMarketplaceLoading.value = false
  }
}

// fetchHomeStats loads aggregate public counters independently from marketplace groups.
// Failure clears the prior statistics and marks their unavailable state. The
// loading flag is released by each request's completion without coordinating overlapping calls.
async function fetchHomeStats() {
  homeStatsLoading.value = true
  homeStatsError.value = false

  try {
    homeStats.value = await getMarketplaceStats()
  } catch (error) {
    // Clear stale totals and mark statistics unavailable so the cards show their
    // error state while a successful marketplace response can still populate provider data.
    console.error('Failed to load home marketplace stats:', error)
    homeStats.value = null
    homeStatsError.value = true
  } finally {
    homeStatsLoading.value = false
  }
}

// Start the tracked icon rotation before awaiting public settings, then request
// both default-page data sources together only when custom home content is absent.
onMounted(async () => {
  authStore.checkAuth()
  homeMarketplaceButtonIconTimer = window.setInterval(() => {
    if (homeMarketplaceButtonBrands.value.length <= 1) {
      return
    }
    homeMarketplaceButtonIconIndex.value += 1
  }, 1800)

  if (!appStore.publicSettingsLoaded) {
    try {
      await appStore.fetchPublicSettings()
    } catch (error) {
      // Continue with the settings already available after reporting the failed
      // refresh, allowing the default page's independent data requests to proceed.
      console.error('Failed to load public settings:', error)
    }
  }

  if (!homeContent.value) {
    await Promise.all([fetchHomeMarketplace(), fetchHomeStats()])
  }
})

// Cancel every tracked count-up frame and the icon interval using their current
// handles; API requests retain their independent completion behavior after teardown.
onUnmounted(() => {
  homeStatAnimationFrames.forEach((frameId) => cancelAnimationFrame(frameId))
  homeStatAnimationFrames.clear()
  if (homeMarketplaceButtonIconTimer) {
    window.clearInterval(homeMarketplaceButtonIconTimer)
    homeMarketplaceButtonIconTimer = null
  }
})
// Locale refresh reloads public model descriptions through the shared subscription;
// this callback retains its existing independence from the custom-home-content decision.
useLocaleRefresh(fetchHomeMarketplace)
</script>

<style scoped>
/* Keep wrapped modality badges aligned with the left column's label. */
.home-featured-modalities {
  justify-content: flex-start;
}

/* The recharge rows, key samples, and their placeholder strokes share the
   reference illustration's tighter corners within this section. */
.home-step-sample,
.home-step-lines::before,
.home-step-lines::after {
  /* check-ui-allow: The 4px radius matches the homepage illustration reference. */
  border-radius: 4px;
}

.home-step-lines::before {
  @apply h-1 bg-gray-950/20 content-[''] dark:bg-dark-50/20;
}

.home-step-lines::after {
  @apply h-0.5 bg-gray-950/[0.08] content-[''] dark:bg-dark-50/[0.08];
}

/* Stretch the native link over the illustration and draw its focus ring inside
   the clipped card. The text remains the link's accessible label. */
.home-feature-link::after {
  @apply absolute inset-0 z-20 content-[''];
}

.home-feature-link:focus-visible::after {
  @apply ring-2 ring-inset ring-primary-600 dark:ring-primary-500;
}

.home-marketplace-icon-enter-active,
.home-marketplace-icon-leave-active {
  transition:
    opacity var(--motion-layout) var(--motion-ease),
    transform var(--motion-layout) var(--motion-ease);
}

.home-marketplace-icon-enter-from {
  opacity: 0;
  transform: translateY(-70%);
}

.home-marketplace-icon-leave-to {
  opacity: 0;
  transform: translateY(70%);
}
</style>
