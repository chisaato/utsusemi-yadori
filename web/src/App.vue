<script setup lang="ts">
import { computed, type Component } from 'vue'
import { NConfigProvider, NDialogProvider, NMessageProvider, darkTheme, dateZhCN, zhCN } from 'naive-ui'
import type { GlobalThemeOverrides } from 'naive-ui'
import { useTheme } from './composables/useTheme'
import AppLayout from './components/AppLayout.vue'

const { isDark } = useTheme()
const theme = computed(() => (isDark.value ? darkTheme : null))

/* 「空蝉」配色：蝉翼薄荷青 accent + 深墨绿底 */
const darkOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#2fd9c3',
    primaryColorHover: '#63efe0',
    primaryColorPressed: '#20ab9b',
    primaryColorSuppl: '#2fd9c3',
    borderRadius: '10px',
    borderRadiusSmall: '8px',
    bodyColor: '#0c1211',
    cardColor: '#131a18',
    modalColor: '#101715',
    popoverColor: '#1a2321',
    inputColor: '#0e1514',
    tableColor: '#131a18',
    borderColor: '#263734',
    dividerColor: '#1e2b29',
  },
}

const lightOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#0a9d8a',
    primaryColorHover: '#0fbfa5',
    primaryColorPressed: '#07796c',
    primaryColorSuppl: '#0a9d8a',
    borderRadius: '10px',
    borderRadiusSmall: '8px',
    bodyColor: '#eef2f1',
    cardColor: '#ffffff',
    modalColor: '#ffffff',
    popoverColor: '#ffffff',
    inputColor: '#f4f7f6',
    tableColor: '#ffffff',
    borderColor: '#d8e2e0',
    dividerColor: '#e3eae8',
  },
}

const overrides = computed<GlobalThemeOverrides>(() => (isDark.value ? darkOverrides : lightOverrides))
</script>

<template>
  <n-config-provider
    :theme="theme"
    :theme-overrides="overrides"
    :locale="zhCN"
    :date-locale="dateZhCN"
    style="height: 100%"
  >
    <n-message-provider placement="top">
      <n-dialog-provider>
        <AppLayout />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
