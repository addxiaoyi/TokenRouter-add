<template>
  <GitHubMark v-if="props.provider === 'github'" aria-hidden="true" focusable="false" />
  <GoogleMark v-else-if="props.provider === 'google'" aria-hidden="true" focusable="false" />
  <LinuxDoMark v-else-if="props.provider === 'linuxdo'" aria-hidden="true" focusable="false" />
  <svg
    v-else-if="props.provider === 'telegram'"
    viewBox="0 0 24 24"
    fill="currentColor"
    aria-hidden="true"
    focusable="false"
  >
    <!-- Verbatim Simple Icons path: https://raw.githubusercontent.com/simple-icons/simple-icons/develop/icons/telegram.svg -->
    <path
      d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"
    />
  </svg>
  <svg
    v-else-if="props.provider === 'discord'"
    viewBox="0 0 24 24"
    fill="currentColor"
    aria-hidden="true"
    focusable="false"
  >
    <!-- Verbatim Simple Icons path: https://raw.githubusercontent.com/simple-icons/simple-icons/develop/icons/discord.svg -->
    <path
      d="M20.317 4.3698a19.7913 19.7913 0 00-4.8851-1.5152.0741.0741 0 00-.0785.0371c-.211.3753-.4447.8648-.6083 1.2495-1.8447-.2762-3.68-.2762-5.4868 0-.1636-.3933-.4058-.8742-.6177-1.2495a.077.077 0 00-.0785-.037 19.7363 19.7363 0 00-4.8852 1.515.0699.0699 0 00-.0321.0277C.5334 9.0458-.319 13.5799.0992 18.0578a.0824.0824 0 00.0312.0561c2.0528 1.5076 4.0413 2.4228 5.9929 3.0294a.0777.0777 0 00.0842-.0276c.4616-.6304.8731-1.2952 1.226-1.9942a.076.076 0 00-.0416-.1057c-.6528-.2476-1.2743-.5495-1.8722-.8923a.077.077 0 01-.0076-.1277c.1258-.0943.2517-.1923.3718-.2914a.0743.0743 0 01.0776-.0105c3.9278 1.7933 8.18 1.7933 12.0614 0a.0739.0739 0 01.0785.0095c.1202.099.246.1981.3728.2924a.077.077 0 01-.0066.1276 12.2986 12.2986 0 01-1.873.8914.0766.0766 0 00-.0407.1067c.3604.698.7719 1.3628 1.225 1.9932a.076.076 0 00.0842.0286c1.961-.6067 3.9495-1.5219 6.0023-3.0294a.077.077 0 00.0313-.0552c.5004-5.177-.8382-9.6739-3.5485-13.6604a.061.061 0 00-.0312-.0286zM8.02 15.3312c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9555-2.4189 2.157-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.9555 2.4189-2.1569 2.4189zm7.9748 0c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9554-2.4189 2.1569-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.946 2.4189-2.1568 2.4189Z"
    />
  </svg>
  <svg
    v-else-if="props.provider === 'wechat'"
    viewBox="0 0 24 24"
    fill="currentColor"
    aria-hidden="true"
    focusable="false"
  >
    <!--
      Verbatim third-party SVG path from Simple Icons:
      https://raw.githubusercontent.com/simple-icons/simple-icons/develop/icons/wechat.svg
    -->
    <path
      d="M8.691 2.188C3.891 2.188 0 5.476 0 9.53c0 2.212 1.17 4.203 3.002 5.55a.59.59 0 0 1 .213.665l-.39 1.48c-.019.07-.048.141-.048.213 0 .163.13.295.29.295a.326.326 0 0 0 .167-.054l1.903-1.114a.864.864 0 0 1 .717-.098 10.16 10.16 0 0 0 2.837.403c.276 0 .543-.027.811-.05-.857-2.578.157-4.972 1.932-6.446 1.703-1.415 3.882-1.98 5.853-1.838-.576-3.583-4.196-6.348-8.596-6.348zM5.785 5.991c.642 0 1.162.529 1.162 1.18a1.17 1.17 0 0 1-1.162 1.178A1.17 1.17 0 0 1 4.623 7.17c0-.651.52-1.18 1.162-1.18zm5.813 0c.642 0 1.162.529 1.162 1.18a1.17 1.17 0 0 1-1.162 1.178 1.17 1.17 0 0 1-1.162-1.178c0-.651.52-1.18 1.162-1.18zm5.34 2.867c-1.797-.052-3.746.512-5.28 1.786-1.72 1.428-2.687 3.72-1.78 6.22.942 2.453 3.666 4.229 6.884 4.229.826 0 1.622-.12 2.361-.336a.722.722 0 0 1 .598.082l1.584.926a.272.272 0 0 0 .14.047c.134 0 .24-.111.24-.247 0-.06-.023-.12-.038-.177l-.327-1.233a.582.582 0 0 1-.023-.156.49.49 0 0 1 .201-.398C23.024 18.48 24 16.82 24 14.98c0-3.21-2.931-5.837-6.656-6.088V8.89c-.135-.01-.27-.027-.407-.03zm-2.53 3.274c.535 0 .969.44.969.982a.976.976 0 0 1-.969.983.976.976 0 0 1-.969-.983c0-.542.434-.982.97-.982zm4.844 0c.535 0 .969.44.969.982a.976.976 0 0 1-.969.983.976.976 0 0 1-.969-.983c0-.542.434-.982.969-.982z"
    />
  </svg>
  <svg
    v-else-if="props.provider === 'dingtalk'"
    viewBox="0 0 24 24"
    fill="currentColor"
    aria-hidden="true"
    focusable="false"
  >
    <!--
      Verbatim third-party SVG path from Remix Icon:
      https://raw.githubusercontent.com/Remix-Design/RemixIcon/master/icons/Logos/dingding-fill.svg
    -->
    <path
      d="M12 2C6.47717 2 2 6.47715 2 12C2 17.5229 6.47717 22 12 22C17.5228 22 22 17.5228 22 12C22 6.47717 17.5229 2 12 2ZM16.4904 11.0399L16.4838 11.0543C16.0641 11.9522 14.9684 13.7132 14.9684 13.7132C14.9684 13.7132 14.9669 13.7091 14.9627 13.7022L14.6424 14.2596H16.1856L13.2383 18.1788L13.9074 15.5127H12.6931L13.115 13.7496C12.7736 13.832 12.3703 13.9451 11.8923 14.0988C11.8923 14.0988 11.2459 14.4774 10.0302 13.3705C10.0302 13.3705 9.21029 12.6482 9.68568 12.4676C9.88788 12.3909 10.6672 12.2935 11.2805 12.2106C12.1092 12.0985 12.619 12.0392 12.619 12.0392C12.619 12.0392 10.0641 12.0775 9.45797 11.9821C8.85178 11.8868 8.08285 10.8753 7.91909 9.98621C7.91909 9.98621 7.66575 9.49805 8.46378 9.72912C9.26178 9.96021 12.5653 10.6286 12.5653 10.6286C12.5653 10.6286 8.26903 9.31176 7.98319 8.99048C7.69727 8.66918 7.14179 7.23638 7.21412 6.35602C7.21412 6.35602 7.24548 6.13656 7.47054 6.19532C7.47054 6.19532 10.6467 7.64667 12.8184 8.44116C14.9902 9.23567 16.8784 9.63987 16.6345 10.6686C16.6136 10.7563 16.5624 10.8848 16.4904 11.0399Z"
    />
  </svg>
  <Icon
    v-else
    :name="props.provider === 'email' ? 'mail' : 'shieldCheck'"
    :animate-on-hover="false"
    aria-hidden="true"
  />
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import GitHubMark from './GitHubMark.vue'
import GoogleMark from '@/components/common/GoogleMark.vue'
import LinuxDoMark from './LinuxDoMark.vue'
import type { UserAuthProvider } from '@/api/identity.types'

// AuthProviderIcon.vue displays decorative identity-provider marks for sign-in
// buttons and account binding rows. GitHub, Google, and Linux DO reuse existing
// brand artwork, while Telegram, Discord, WeChat, and DingTalk use upstream SVG
// paths that inherit the caller's text color. Email and OIDC use shared symbols;
// the parent owns sizing, provider labels, availability, and all interactions.

// props selects provider artwork without changing binding availability or introducing
// controls; callers provide the visible provider label beside this decorative mark.
const props = defineProps<{
  provider: UserAuthProvider // Sign-in provider represented by the decorative mark.
}>()
</script>
