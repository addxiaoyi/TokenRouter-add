<template>
  <div class="space-y-4">
    <div>
      <label for="sms-phone" class="input-label">
        {{ t('auth.sms.phoneLabel') }}
      </label>
      <div class="input-icon-wrap input-icon-lg">
        <div class="input-icon">
          <Icon name="chat" size="md" class="text-gray-400 dark:text-dark-500" />
        </div>
        <input
          id="sms-phone"
          v-model="phone"
          type="tel"
          autocomplete="tel"
          :disabled="disabled"
          class="input input-has-icon"
          :class="{ 'input-error': phoneError }"
          :placeholder="t('auth.sms.phonePlaceholder')"
        />
      </div>
      <p v-if="phoneError" class="mt-1 text-sm text-red-500">{{ phoneError }}</p>
    </div>

    <div>
      <label for="sms-code" class="input-label">
        {{ t('auth.sms.codeLabel') }}
      </label>
      <div class="flex gap-3">
        <div class="input-icon-wrap input-icon-lg flex-1">
          <div class="input-icon">
            <Icon name="key" size="md" class="text-gray-400 dark:text-dark-500" />
          </div>
          <input
            id="sms-code"
            v-model="code"
            type="text"
            inputmode="numeric"
            autocomplete="one-time-code"
            :disabled="disabled"
            class="input input-has-icon"
            :placeholder="t('auth.sms.codePlaceholder')"
          />
        </div>
        <button
          type="button"
          class="btn btn-secondary whitespace-nowrap"
          :disabled="disabled || countdown > 0 || !phoneValid"
          @click="handleSendCode"
        >
          {{
            countdown > 0 ? t('auth.sms.resendIn', { seconds: countdown }) : t('auth.sms.sendCode')
          }}
        </button>
      </div>
    </div>

    <button
      type="button"
      class="btn btn-primary w-full"
      :disabled="disabled || !phoneValid || !code.trim()"
      @click="handleLogin"
    >
      {{ t('auth.sms.signIn') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

// SMSLoginSection.vue collects a phone number and its verification code for
// SMS sign-in. The parent owns captcha proof, the send and login requests,
// and every outcome (session, TOTP challenge, or pending registration); this
// section owns only the draft inputs and the resend countdown, which starts
// optimistically when a send is requested.

// props supplies parent-owned configuration. emit reports the send and login
// intents back to that owner with the current draft values.
const props = defineProps<{
  disabled?: boolean // parent action disable flag
}>() // parent component inputs
const emit = defineEmits<{
  'send-code': [phone: string] // parent code-send request
  login: [payload: { phone: string; code: string }] // parent code-login request
}>() // parent component event dispatcher

const { t } = useI18n() // translated authentication message resolver

const phone = ref<string>('') // entered mobile number draft
const code = ref<string>('') // entered verification code draft
const countdown = ref<number>(0) // resend countdown seconds remaining
let countdownTimer: ReturnType<typeof setInterval> | null = null // active countdown interval

// phoneValid applies the same mainland-mobile rule the server enforces, so
// the send action stays disabled for shapes the server would reject.
const phoneValid = computed(() => /^1[3-9]\d{9}$/.test(phone.value.trim())) // draft number validity

// phoneError surfaces the invalid-number message only after the user typed
// a nonempty value that fails the rule.
const phoneError = computed(() =>
  phone.value.trim() && !phoneValid.value ? t('auth.sms.invalidPhone') : '',
) // draft number validation message

// handleSendCode emits the send request and starts the resend countdown.
// The countdown is optimistic: a failed send still surfaces through the
// parent's error reporting, and the countdown simply bounds retry rate.
function handleSendCode(): void {
  if (props.disabled || countdown.value > 0 || !phoneValid.value) return
  emit('send-code', phone.value.trim())
  countdown.value = 60
  countdownTimer = setInterval(() => {
    countdown.value -= 1
    if (countdown.value <= 0 && countdownTimer) {
      clearInterval(countdownTimer)
      countdownTimer = null
    }
  }, 1000)
}

// handleLogin emits the login request with the trimmed draft values.
function handleLogin(): void {
  if (props.disabled || !phoneValid.value || !code.value.trim()) return
  emit('login', { phone: phone.value.trim(), code: code.value.trim() })
}

onBeforeUnmount(() => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
})
</script>
