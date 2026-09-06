<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { NButton, NIcon, NTooltip, useDialog, useMessage } from 'naive-ui'
import {
  AppsOutline,
  ContrastOutline,
  CubeOutline,
  HomeOutline,
  MoonOutline,
  QrCodeOutline,
  SunnyOutline,
  TerminalOutline,
} from '@vicons/ionicons5'
import type { Component } from 'vue'
import { transportLabel } from '../api/client'
import { useTheme } from '../composables/useTheme'

const route = useRoute()
const message = useMessage()
const dialog = useDialog()
const { pref, cycle } = useTheme()

interface NavItem {
  key: string
  to: string
  label: string
  icon: Component
}

const navItems: NavItem[] = [
  { key: 'overview', to: '/', label: '总览', icon: HomeOutline },
  { key: 'apps', to: '/apps', label: '注入', icon: AppsOutline },
  { key: 'bin', to: '/bin', label: '二进制', icon: CubeOutline },
  { key: 'remote', to: '/remote', label: '远程', icon: QrCodeOutline },
  { key: 'logs', to: '/logs', label: '日志', icon: TerminalOutline },
]

const activeKey = computed(() => String(route.name ?? ''))

/* ---------------- 主题三态循环 ---------------- */

const THEME_LABEL = { auto: '跟随系统', dark: '深色', light: '浅色' } as const
const THEME_ICON = { auto: ContrastOutline, dark: MoonOutline, light: SunnyOutline } as const

const themeIcon = computed(() => THEME_ICON[pref.value])

function onCycle() {
  cycle()
  message.info(`主题：${THEME_LABEL[pref.value]}`, { duration: 1400 })
}

/* ---------------- 传输模式徽标 ---------------- */

const chipTitle = computed(() => {
  if (transportLabel === 'MOCK') return '开发模式：内置模拟数据（不进产物）'
  if (transportLabel === 'KSU') return 'KSU 管理器内置模式（utsusemi-ctl api）'
  return 'PC 远程模式（REST + Bearer token）'
})

/* ---------------- 401 全局提示（契约 §5） ---------------- */

let authDialogOpen = false

function onUnauthorized() {
  if (authDialogOpen) return
  authDialogOpen = true
  const d = dialog.warning({
    title: '登录已失效',
    content: '访问令牌无效或已过期。请从带 ?token= 的入口链接重新打开此页面。',
    positiveText: '知道了',
    onClose: () => {
      authDialogOpen = false
    },
    onPositiveClick: () => {
      authDialogOpen = false
    },
  })
  void d
}

onMounted(() => window.addEventListener('utsusemi:unauthorized', onUnauthorized))
onUnmounted(() => window.removeEventListener('utsusemi:unauthorized', onUnauthorized))
</script>

<template>
  <div class="shell">
    <div class="bg-fx" aria-hidden="true"></div>

    <!-- 桌面侧栏 -->
    <aside class="rail">
      <RouterLink to="/" class="brand" style="text-decoration: none; color: inherit">
        <span class="brand-name">空蝉宿</span>
        <span class="brand-sub">utsusemi</span>
      </RouterLink>
      <nav class="rail-nav">
        <RouterLink
          v-for="item in navItems"
          :key="item.key"
          :to="item.to"
          class="rail-item"
          :class="{ active: activeKey === item.key }"
        >
          <n-icon :component="item.icon" :size="18" />
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>
      <div class="rail-foot">
        <span class="transport-chip mono" :title="chipTitle">{{ transportLabel }}</span>
        <n-tooltip placement="top-end">
          <template #trigger>
            <n-button quaternary circle size="small" @click="onCycle">
              <template #icon>
                <n-icon :component="themeIcon" />
              </template>
            </n-button>
          </template>
          主题：{{ THEME_LABEL[pref] }}（点击切换）
        </n-tooltip>
      </div>
    </aside>

    <div class="main-col">
      <!-- 移动端顶栏 -->
      <header class="topbar">
        <RouterLink to="/" class="brand" style="text-decoration: none; color: inherit">
          <span class="brand-name">空蝉宿</span>
          <span class="brand-sub">utsusemi</span>
        </RouterLink>
        <div style="display: flex; align-items: center; gap: 8px">
          <span class="transport-chip mono" :title="chipTitle">{{ transportLabel }}</span>
          <n-button quaternary circle size="small" @click="onCycle">
            <template #icon>
              <n-icon :component="themeIcon" />
            </template>
          </n-button>
        </div>
      </header>

      <main class="content">
        <RouterView v-slot="{ Component }">
          <Transition name="page" mode="out-in">
            <component :is="Component" />
          </Transition>
        </RouterView>
      </main>

      <!-- 移动端底部 Tab -->
      <nav class="tabbar">
        <RouterLink
          v-for="item in navItems"
          :key="item.key"
          :to="item.to"
          class="tab-item"
          :class="{ active: activeKey === item.key }"
        >
          <n-icon :component="item.icon" :size="20" />
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>
    </div>
  </div>
</template>
