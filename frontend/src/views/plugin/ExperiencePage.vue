<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { readExperience, type ExperienceStatus } from '@/api/experience'
import Icon from '@/components/icons/Icon.vue'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import { useAppStore } from '@/stores/app'

// ExperiencePage.vue presents cumulative check-in and invitation experience as
// membership progress. The main application's authenticated experience endpoint
// owns level thresholds and Free-group eligibility, while this page explains daily
// rewards, the next level, and growth stages with the application's shared controls.

// Stage thresholds select milestone translation keys; the server response determines
// Free-group eligibility while higher stages introduce names without additional privileges.
const stages = [
  { level: 0, key: 'newArrival' },
  { level: 1, key: 'firstLight' },
  { level: 3, key: 'findingYourStride' },
  { level: 5, key: 'alongTheWay' },
  { level: 10, key: 'amongTheStars' },
] as const
// Shared public settings gate the affiliate route, while server progress and locale
// supply the page display; this mount owns and cancels its experience request.
const appStore = useAppStore()
const { t } = useI18n()
const status = ref<ExperienceStatus | null>(null)
const loading = ref(false)
const failed = ref(false)
const request = new AbortController()
const freeUnlocked = computed(() => status.value?.free_unlocked === true)
// The affiliate route guard only admits visitors when the shared setting is explicitly true.
const affiliateEnabled = computed(() => appStore.cachedPublicSettings?.affiliate_enabled === true)
const affiliateSettingsUnknown = computed(() => !appStore.publicSettingsLoaded)
const currentStage = computed(
  () =>
    [...stages].reverse().find((stage) => (status.value?.level ?? 0) >= stage.level) ?? stages[0],
)
const nextStage = computed(() => stages.find((stage) => stage.level > (status.value?.level ?? 0)))

/** refresh reads and displays the signed-in user's cumulative experience progress.
 * It ignores a second refresh while one request is active; failures retain the last
 * successful status and expose a retry message. The page owns its AbortController,
 * so unmount cancellation prevents late responses and errors from publishing state
 * after the page has retired.
 */
async function refresh(): Promise<void> {
  if (loading.value) return
  loading.value = true
  failed.value = false
  try {
    const progress = await readExperience(request.signal)
    if (!request.signal.aborted) status.value = progress
  } catch {
    if (!request.signal.aborted) failed.value = true
  } finally {
    if (!request.signal.aborted) loading.value = false
  }
}

/** loadAffiliateSettings reads shared feature flags before exposing a guarded destination. */
async function loadAffiliateSettings(): Promise<void> {
  if (!appStore.publicSettingsLoaded) await appStore.fetchPublicSettings()
}

onMounted(() => {
  void loadAffiliateSettings()
  void refresh()
})
onBeforeUnmount(() => request.abort())
</script>

<template>
  <section class="experience-page" :aria-busy="loading">
    <header class="experience-heading">
      <div>
        <h1 class="page-title">{{ t('experience.title') }}</h1>
        <p>{{ t('experience.description') }}</p>
      </div>
      <button class="btn btn-secondary" type="button" :disabled="loading" @click="refresh">
        <Icon name="refresh" size="sm" />
        {{ loading ? t('experience.updating') : t('experience.refresh') }}
      </button>
    </header>

    <p v-if="failed" class="experience-error" role="alert">
      {{ t('experience.loadFailed') }}
    </p>
    <ContentSkeleton v-if="!status && loading" variant="detail" :rows="6" />

    <template v-if="status">
      <section class="experience-overview" :aria-label="t('experience.currentLevel')">
        <div class="experience-identity">
          <div class="experience-emblem" aria-hidden="true">
            <Icon name="crown" size="lg" />
            <span><small>Lv.</small>{{ status.level }}</span>
          </div>
          <div class="experience-identity-copy">
            <span class="experience-eyebrow">{{ t('experience.currentStage') }}</span>
            <h2>{{ t(`experience.stages.${currentStage.key}`) }}</h2>
            <p>{{
              nextStage
                ? t('experience.nextStage', {
                    level: nextStage.level,
                    stage: t(`experience.stages.${nextStage.key}`),
                  })
                : t('experience.keepEarning')
            }}</p>
          </div>
          <span class="experience-access-label">
            <Icon :name="freeUnlocked ? 'checkCircle' : 'lock'" size="sm" />
            {{
              freeUnlocked ? t('experience.freeRequirementMet') : t('experience.freeRequirement')
            }}
          </span>
        </div>
        <div class="experience-level-progress">
          <div class="experience-progress-label">
            <span>{{ t('experience.currentLevelProgress') }}</span>
            <strong>{{ status.level_exp }} / {{ status.level_required_exp }} XP</strong>
          </div>
          <progress
            class="experience-progress"
            :value="status.level_exp"
            :max="status.level_required_exp"
            :aria-label="t('experience.levelExperienceProgress')"
          />
          <p class="experience-help">{{
            t('experience.levelProgress', {
              level: status.level + 1,
              expToNext: status.exp_to_next,
              nextLevelExp: status.next_level_exp,
            })
          }}</p>
        </div>
        <dl class="experience-stats">
          <div>
            <dt>{{ t('experience.experienceEarned') }}</dt>
            <dd>{{ status.exp }}<small>XP</small></dd>
          </div>
          <div>
            <dt>{{ t('experience.currentStreak') }}</dt>
            <dd
              >{{ status.streak }}<small>{{ t('experience.days') }}</small></dd
            >
          </div>
          <div>
            <dt>{{ t('experience.totalCheckIns') }}</dt>
            <dd
              >{{ status.total }}<small>{{ t('experience.days') }}</small></dd
            >
          </div>
        </dl>
      </section>

      <div class="experience-details">
        <section class="experience-panel">
          <div class="experience-panel-heading">
            <span class="experience-icon"><Icon name="calendar" size="md" /></span>
            <h2>{{ t('experience.dailyCheckIn') }}</h2>
            <span class="experience-point">{{ t('experience.xpPerDay') }}</span>
          </div>
          <p class="experience-goal">{{
            status.checked_in_today ? t('experience.checkedInToday') : t('experience.checkInToday')
          }}</p>
          <div class="experience-progress-label">
            <span>{{ t('experience.checkInExperience') }}</span>
            <strong>{{ status.checkin_exp }} XP</strong>
          </div>
          <p class="experience-help">{{ t('experience.checkInHelp') }}</p>
          <RouterLink class="btn btn-secondary" to="/ext/tokenflux.checkin">
            {{ status.checked_in_today ? t('experience.viewCheckIns') : t('experience.goToCheckIn')
            }}<Icon name="arrowRight" size="sm" />
          </RouterLink>
        </section>

        <section class="experience-panel">
          <div class="experience-panel-heading">
            <span class="experience-icon"><Icon name="userPlus" size="md" /></span>
            <h2>{{ t('experience.inviteFriends') }}</h2>
            <span class="experience-point">{{ t('experience.xpPerFriend') }}</span>
          </div>
          <p class="experience-goal">{{ t('experience.inviteGoal') }}</p>
          <div class="experience-progress-label">
            <span>{{ t('experience.inviteExperience') }}</span>
            <strong>{{ status.invite_exp }} XP</strong>
          </div>
          <p class="experience-help">{{ t('experience.inviteHelp') }}</p>
          <RouterLink v-if="affiliateEnabled" class="btn btn-secondary" to="/affiliate">
            {{ t('experience.inviteFriend') }}<Icon name="arrowRight" size="sm" />
          </RouterLink>
          <div v-else class="experience-locked-note" role="status">
            <Icon name="lock" size="sm" />
            {{
              affiliateSettingsUnknown
                ? t('experience.inviteAvailabilityUnknown')
                : t('experience.invitationsUnavailable')
            }}
          </div>
        </section>

        <section class="experience-panel experience-benefit">
          <div class="experience-panel-heading">
            <span class="experience-icon"
              ><Icon :name="freeUnlocked ? 'checkCircle' : 'lock'" size="md"
            /></span>
            <h2>{{ t('experience.freeGroupAccess') }}</h2>
            <span class="experience-point">Lv.1+</span>
          </div>
          <p class="experience-goal">{{
            freeUnlocked ? t('experience.levelRequirementMet') : t('experience.reachLevel')
          }}</p>
          <p class="experience-help">{{
            freeUnlocked ? t('experience.freeHelpUnlocked') : t('experience.freeHelpLocked')
          }}</p>
          <RouterLink v-if="freeUnlocked" class="btn btn-primary" to="/keys">
            {{ t('experience.configureApiKey') }}<Icon name="arrowRight" size="sm" />
          </RouterLink>
          <div v-else class="experience-locked-note">
            <Icon name="lock" size="sm" />{{ t('experience.earnExperience') }}
          </div>
        </section>
      </div>

      <section class="experience-journey" :aria-labelledby="'experience-stages-title'">
        <div class="experience-journey-heading">
          <h2 id="experience-stages-title">{{ t('experience.milestones') }}</h2>
          <p>{{ t('experience.milestonesIntro') }}</p>
        </div>
        <ol class="experience-stages">
          <li
            v-for="stage in stages"
            :key="stage.level"
            :class="{
              'is-current': stage.level === currentStage.level,
              'is-reached': status.level >= stage.level,
            }"
            :aria-current="stage.level === currentStage.level ? 'step' : undefined"
          >
            <span class="experience-stage-marker"
              ><Icon :name="status.level >= stage.level ? 'check' : 'lock'" size="sm"
            /></span>
            <span class="experience-stage-level"
              >Lv.{{ stage.level }}{{ stage.level === 10 ? '+' : '' }}</span
            >
            <h3>{{ t(`experience.stages.${stage.key}`) }}</h3>
            <p>{{
              stage.level === 1
                ? t('experience.unlockFreeGroups')
                : stage.level === currentStage.level
                  ? t('experience.currentStageStatus')
                  : status.level >= stage.level
                    ? t('experience.reached')
                    : t('experience.aheadOfYou')
            }}</p>
          </li>
        </ol>
      </section>
    </template>
  </section>
</template>
