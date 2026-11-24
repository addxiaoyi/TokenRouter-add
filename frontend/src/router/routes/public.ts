import type { RouteRecordRaw } from 'vue-router'

// public.ts declares setup, sign-in, callback, and public information pages with lazy component loaders.
// Route metadata supplies titles and access flags to the shared guards and
// layouts; records retain their declaration order when assembled by the router.

// publicRoutes supplies the ordered records for this navigation area to the application router.
export const publicRoutes: RouteRecordRaw[] = [
  {
    path: '/docs',
    name: 'ApiDocs',
    component: () => import('@/views/public/ApiDocsView.vue'),
    meta: { requiresAuth: false, titleKey: 'apiDocs.title' },
  },
  {
    path: '/model-quality',
    name: 'ModelQuality',
    component: () => import('@/views/ModelQualityView.vue'),
    meta: { requiresAuth: false, titleKey: 'quality.title', descriptionKey: 'quality.description' },
  },
  {
    path: '/setup',
    name: 'Setup',
    component: () => import('@/views/setup/SetupWizardView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Setup',
    },
  },

  {
    path: '/home',
    name: 'Home',
    component: () => import('@/views/HomeView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Home',
    },
  },
  {
    path: '/models',
    alias: '/pricing',
    name: 'ModelMarketplace',
    component: () => import('@/views/ModelMarketplaceView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Model Marketplace',
      titleKey: 'marketplace.title',
      descriptionKey: 'marketplace.subtitle',
    },
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/auth/LoginView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Login',
      titleKey: 'common.login',
    },
  },
  {
    path: '/register',
    name: 'Register',
    component: () => import('@/views/auth/RegisterView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Register',
      titleKey: 'auth.createAccount',
    },
  },
  {
    path: '/email-verify',
    name: 'EmailVerify',
    component: () => import('@/views/auth/EmailVerifyView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Verify Email',
    },
  },
  {
    path: '/auth/callback',
    name: 'OAuthCallback',
    component: () => import('@/views/auth/OAuthCallbackView.vue'),
    meta: {
      requiresAuth: false,
      title: 'OAuth Callback',
      titleKey: 'auth.oauthCallbackPageTitle',
    },
  },
  {
    path: '/auth/oauth/callback',
    name: 'EmailOAuthCallback',
    component: () => import('@/views/auth/OAuthCallbackView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Email OAuth Callback',
      titleKey: 'auth.oauthCallbackPageTitle',
    },
  },
  {
    path: '/auth/linuxdo/callback',
    name: 'LinuxDoOAuthCallback',
    component: () => import('@/views/auth/LinuxDoCallbackView.vue'),
    meta: {
      requiresAuth: false,
      title: 'LinuxDo OAuth Callback',
      titleKey: 'auth.linuxdoCallbackPageTitle',
    },
  },
  {
    path: '/auth/qq/callback',
    name: 'QQOAuthCallback',
    component: () => import('@/views/auth/QQCallbackView.vue'),
    meta: {
      requiresAuth: false,
      title: 'QQ OAuth Callback',
      titleKey: 'auth.qqCallbackPageTitle',
    },
  },
  {
    path: '/auth/phone/callback',
    name: 'PhoneAuthCallback',
    component: () => import('@/views/auth/PhoneCallbackView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Phone Auth Callback',
      titleKey: 'auth.phoneCallbackPageTitle',
    },
  },
  {
    path: '/auth/wechat/callback',
    name: 'WeChatOAuthCallback',
    component: () => import('@/views/auth/WechatCallbackView.vue'),
    meta: {
      requiresAuth: false,
      title: 'WeChat OAuth Callback',
      titleKey: 'auth.wechatCallbackPageTitle',
    },
  },
  {
    path: '/auth/wechat/payment/callback',
    name: 'WeChatPaymentOAuthCallback',
    component: () => import('@/views/auth/WechatPaymentCallbackView.vue'),
    meta: {
      requiresAuth: false,
      title: 'WeChat Payment Callback',
      titleKey: 'auth.wechatPaymentCallbackPageTitle',
    },
  },
  {
    path: '/auth/dingtalk/callback',
    name: 'DingTalkOAuthCallback',
    component: () => import('@/views/auth/DingTalkCallbackView.vue'),
    meta: {
      requiresAuth: false,
      title: 'DingTalk OAuth Callback',
      titleKey: 'auth.dingtalkCallbackPageTitle',
    },
  },
  {
    path: '/auth/dingtalk/email-completion',
    name: 'dingtalk-email-completion',
    component: () => import('@/views/auth/DingTalkEmailCompletionView.vue'),
    meta: {
      requiresAuth: false,
      title: 'DingTalk Email Completion',
    },
  },
  {
    path: '/auth/telegram/callback',
    name: 'telegramOAuthCallback',
    component: () => import('@/views/auth/OidcCallbackView.vue'),
    props: { provider: 'telegram' },
    meta: { requiresAuth: false, title: 'Telegram Login', titleKey: 'auth.social.callbackTitle' },
  },
  {
    path: '/auth/discord/callback',
    name: 'discordOAuthCallback',
    component: () => import('@/views/auth/OidcCallbackView.vue'),
    props: { provider: 'discord' },
    meta: { requiresAuth: false, title: 'Discord Login', titleKey: 'auth.social.callbackTitle' },
  },
  {
    path: '/auth/oidc/callback',
    name: 'OIDCOAuthCallback',
    component: () => import('@/views/auth/OidcCallbackView.vue'),
    meta: {
      requiresAuth: false,
      title: 'OIDC OAuth Callback',
      titleKey: 'auth.oidcCallbackPageTitle',
    },
  },
  {
    path: '/forgot-password',
    name: 'ForgotPassword',
    component: () => import('@/views/auth/ForgotPasswordView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Forgot Password',
      titleKey: 'auth.forgotPasswordTitle',
    },
  },
  {
    path: '/reset-password',
    name: 'ResetPassword',
    component: () => import('@/views/auth/ResetPasswordView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Reset Password',
    },
  },
  {
    path: '/key-usage',
    name: 'KeyUsage',
    component: () => import('@/views/KeyUsageView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Key Usage',
    },
  },
  {
    path: '/legal/:documentId',
    name: 'LegalDocument',
    component: () => import('@/views/public/LegalDocumentView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Legal Document',
    },
  },
]

// notFoundRoute remains last in the assembled records and uses the default authentication requirement.
export const notFoundRoute: RouteRecordRaw = {
  path: '/:pathMatch(.*)*',
  name: 'NotFound',
  component: () => import('@/views/NotFoundView.vue'),
  meta: {
    title: '404 Not Found',
  },
}
