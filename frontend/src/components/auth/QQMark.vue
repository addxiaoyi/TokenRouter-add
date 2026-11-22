<!--
  QQMark.vue renders the QQ sign-in mark in the auth button. The mark owns
  the badge shape and brand color. Callers set its size and provide any
  accessible label.
-->
<template>
  <svg viewBox="0 0 24 24" aria-hidden="true">
    <circle cx="12" cy="12" r="10" fill="#12B7F5" />
    <text
      x="12"
      y="15.5"
      text-anchor="middle"
      font-family="Arial, Helvetica, sans-serif"
      font-size="8"
      font-weight="700"
      fill="#FFFFFF"
    >
      QQ
    </text>
  </svg>
</template>
