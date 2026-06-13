<script setup lang="ts">
import {
  NConfigProvider, NMessageProvider, NNotificationProvider, NDialogProvider,
  NGlobalStyle, zhCN, dateZhCN,
} from 'naive-ui'
import { themeOverrides } from './theme'
</script>

<template>
  <n-config-provider :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <n-global-style />
    <n-message-provider>
      <n-notification-provider :max="4" container-class="toast-compact">
        <n-dialog-provider>
          <router-view />
        </n-dialog-provider>
      </n-notification-provider>
    </n-message-provider>
  </n-config-provider>
</template>
