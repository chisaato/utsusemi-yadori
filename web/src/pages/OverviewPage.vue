<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import {
  NAlert,
  NButton,
  NDynamicTags,
  NIcon,
  NPopconfirm,
  NSelect,
  NSpin,
  NSwitch,
  useMessage,
} from 'naive-ui'
import {
  ChevronForwardOutline,
  PlayOutline,
  ReloadOutline,
  RefreshOutline,
  SettingsOutline,
  StopOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import PageHead from '../components/PageHead.vue'
import { serverRestart, serverSet, serverStart, serverStop } from '../api'
import type { ServerSettings } from '../api/types'
import { useStatusStore } from '../composables/useStatusStore'
import { errMsg, humanDuration } from '../utils/format'

const message = useMessage()
const { status, loading, error, fetchedAt, refresh } = useStatusStore()

onMounted(() => {
  // 每次进入页面拉新；store 为单例，其他页面回来时也能复用
  if (!loading.value) refresh()
})

/* ---------------- 运行时长实时跳动 ---------------- */

const tick = ref(0)
let timer = 0 as unknown as ReturnType<typeof setInterval>
onMounted(() => {
  timer = setInterval(() => tick.value++, 1000)
})
onUnmounted(() => clearInterval(timer))

const serverOn = computed(() => status.value?.server.running ?? false)

const uptimeText = computed(() => {
  void tick.value
  if (!status.value?.server.running) return '—'
  const base = status.value.server.uptime_sec ?? 0
  const live = fetchedAt.value ? base + (Date.now() - fetchedAt.value) / 1000 : base
  return humanDuration(live)
})

/* 激活的 server 二进制元数据（版本展示用） */
const activeServerMeta = computed(() => {
  const s = status.value
  if (!s) return null
  return s.servers.find((b) => b.file === s.settings.server.active) ?? null
})

const rulesStat = computed(() => {
  const rules = status.value?.rules.rules ?? []
  return {
    total: rules.length,
    enabled: rules.filter((r) => r.enabled).length,
    gating: rules.filter((r) => r.child_gating_enabled).length,
  }
})

/* ---------------- 一键启停 ---------------- */

type Busy = 'start' | 'stop' | 'restart' | 'save' | null
const busy = ref<Busy>(null)

async function act(kind: Exclude<Busy, null | 'save'>) {
  if (busy.value) return
  busy.value = kind
  try {
    if (kind === 'start') await serverStart()
    else if (kind === 'stop') await serverStop()
    else await serverRestart()
    message.success(kind === 'start' ? 'server 已启动' : kind === 'stop' ? 'server 已停止' : 'server 已重启')
    await refresh()
  } catch (e) {
    message.error(errMsg(e))
    refresh() // 失败也刷新，让界面回到真实状态
  } finally {
    busy.value = null
  }
}

/* ---------------- server 设置（PUT /api/server 全量替换） ---------------- */

const showSettings = ref(false)
const editing = ref<ServerSettings | null>(null)

watch(
  () => status.value,
  (s) => {
    // 首次载入后建立可编辑副本；此后刷新不覆盖用户编辑中的表单
    if (s && !editing.value) editing.value = JSON.parse(JSON.stringify(s.settings.server))
  },
  { immediate: true },
)

const dirty = computed(
  () =>
    !!editing.value &&
    !!status.value &&
    JSON.stringify(editing.value) !== JSON.stringify(status.value.settings.server),
)

const serverOptions = computed(() =>
  (status.value?.servers ?? []).map((b) => ({
    label: `${b.variant} ${b.version} · ${b.arch}${b.missing ? ' · 文件丢失' : ''}`,
    value: b.file,
  })),
)

function resetEditing() {
  if (status.value) editing.value = JSON.parse(JSON.stringify(status.value.settings.server))
}

async function saveSettings() {
  if (!editing.value || !dirty.value || busy.value) return
  busy.value = 'save'
  try {
    await serverSet(editing.value)
    message.success('设置已保存')
    await refresh()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    busy.value = null
  }
}
</script>

<template>
  <div class="page">
    <PageHead title="总览" desc="server 运行状态与模块概览">
      <template #actions>
        <NButton size="small" quaternary circle :loading="loading" @click="refresh()">
          <template #icon>
            <NIcon><RefreshOutline /></NIcon>
          </template>
        </NButton>
      </template>
    </PageHead>

    <!-- 首次加载失败 -->
    <div v-if="error && !status" class="card reveal" style="--i: 1">
      <div class="card-head">
        <span class="sec-label">status</span>
        <NIcon color="var(--ux-danger)"><WarningOutline /></NIcon>
      </div>
      <p class="error-text">{{ error }}</p>
      <NButton size="small" secondary @click="refresh()">重试</NButton>
    </div>

    <div v-else-if="!status" class="page-loading">
      <NSpin size="large" />
    </div>

    <template v-else>
      <!-- server 运行态 -->
      <section class="card reveal" style="--i: 1">
        <div class="card-head">
          <span class="sec-label">frida server</span>
          <span class="state-badge" :class="serverOn ? 'on' : 'off'">
            <i class="dot" />
            {{ serverOn ? '运行中' : '已停止' }}
          </span>
        </div>

        <div class="server-binary">
          {{ status.server.binary || status.settings.server.active || '未设置激活二进制' }}
        </div>

        <!-- note 非空 = 异常原因（契约 §4.1） -->
        <NAlert v-if="status.server.note" type="error" :bordered="false" style="margin: 6px 0">
          {{ status.server.note }}
        </NAlert>

        <div class="stat-grid">
          <div class="stat">
            <span class="stat-num">{{ serverOn ? (status.server.pid ?? '—') : '—' }}</span>
            <span class="stat-label">PID</span>
          </div>
          <div class="stat">
            <span class="stat-num">{{ uptimeText }}</span>
            <span class="stat-label">运行时长</span>
          </div>
          <div class="stat">
            <span class="stat-num">{{ activeServerMeta ? activeServerMeta.version : '—' }}</span>
            <span class="stat-label">激活版本</span>
          </div>
        </div>

        <div class="action-row">
          <NButton type="primary" :disabled="serverOn" :loading="busy === 'start'" @click="act('start')">
            <template #icon><NIcon><PlayOutline /></NIcon></template>
            启动
          </NButton>

          <NPopconfirm @positive-click="act('stop')">
            <template #trigger>
              <NButton type="error" secondary :disabled="!serverOn" :loading="busy === 'stop'">
                <template #icon><NIcon><StopOutline /></NIcon></template>
                停止
              </NButton>
            </template>
            确认停止 server？运行中应用的注入将失效。
          </NPopconfirm>

          <NButton secondary :disabled="!serverOn" :loading="busy === 'restart'" @click="act('restart')">
            <template #icon><NIcon><ReloadOutline /></NIcon></template>
            重启
          </NButton>
        </div>
      </section>

      <div class="grid-2">
        <!-- 激活版本 -->
        <section class="card reveal" style="--i: 2">
          <div class="card-head">
            <span class="sec-label">激活版本</span>
            <RouterLink to="/bin" class="link-more">
              管理 <NIcon :size="14"><ChevronForwardOutline /></NIcon>
            </RouterLink>
          </div>
          <div class="kv">
            <span class="kv-key">server</span>
            <span class="kv-val">{{ status.settings.server.active || '未设置' }}</span>
          </div>
          <div class="kv">
            <span class="kv-key">gadget</span>
            <span class="kv-val">{{ status.settings.gadget.active || '未设置' }}</span>
          </div>
          <div class="kv">
            <span class="kv-key">启动参数</span>
            <span class="kv-val">{{ status.settings.server.args.join(' ') || '默认' }}</span>
          </div>
        </section>

        <!-- 规则摘要 -->
        <section class="card reveal" style="--i: 3">
          <div class="card-head">
            <span class="sec-label">注入规则</span>
            <RouterLink to="/apps" class="link-more">
              配置 <NIcon :size="14"><ChevronForwardOutline /></NIcon>
            </RouterLink>
          </div>
          <div class="stat-grid" style="margin: 0 0 10px">
            <div class="stat">
              <span class="stat-num">{{ rulesStat.enabled }}</span>
              <span class="stat-label">已启用</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ rulesStat.total }}</span>
              <span class="stat-label">规则总数</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ rulesStat.gating }}</span>
              <span class="stat-label">子进程管控</span>
            </div>
          </div>
        </section>
      </div>

      <!-- web 服务信息 -->
      <section class="card reveal" style="--i: 4">
        <div class="card-head">
          <span class="sec-label">web 远程服务</span>
          <RouterLink to="/remote" class="link-more">
            管理 <NIcon :size="14"><ChevronForwardOutline /></NIcon>
          </RouterLink>
        </div>
        <div class="kv">
          <span class="kv-key">状态</span>
          <span class="state-badge" :class="status.web.enabled ? 'on' : 'off'" style="font-size: 12px">
            <i class="dot" />
            {{ status.web.enabled ? '已开启' : '未开启' }}
          </span>
        </div>
        <div class="kv">
          <span class="kv-key">端口</span>
          <span class="kv-val">{{ status.web.port }}</span>
        </div>
        <div class="kv">
          <span class="kv-key">访问令牌</span>
          <span class="kv-val">{{ status.web.token ? status.web.token.slice(0, 4) + '····' : '—' }}</span>
        </div>
      </section>

      <!-- server 设置 -->
      <section class="card reveal" style="--i: 5">
        <div class="card-head" style="cursor: pointer; user-select: none" @click="showSettings = !showSettings">
          <span class="sec-label" style="display: inline-flex; align-items: center; gap: 6px">
            <NIcon :size="13"><SettingsOutline /></NIcon>
            server 设置
          </span>
          <span class="mono" style="font-size: 11px; color: var(--ux-text-faint)">
            {{ showSettings ? '收起 ▴' : '展开 ▾' }}
          </span>
        </div>

        <div v-if="showSettings && editing" class="form-grid two">
          <div class="form-item">
            <span class="form-label">开机自启</span>
            <NSwitch v-model:value="editing.autostart" size="small" />
          </div>
          <div class="form-item">
            <span class="form-label">崩溃自动重启</span>
            <NSwitch v-model:value="editing.restart_on_crash" size="small" />
          </div>
          <div class="form-item" style="grid-column: 1 / -1">
            <span class="form-label">激活 server 二进制</span>
            <NSelect v-model:value="editing.active" :options="serverOptions" size="small" />
          </div>
          <div class="form-item" style="grid-column: 1 / -1">
            <span class="form-label">启动参数（每个标签一个 arg）</span>
            <NDynamicTags v-model:value="editing.args" size="small" />
          </div>
          <div class="action-row" style="grid-column: 1 / -1">
            <NButton
              size="small"
              type="primary"
              :disabled="!dirty"
              :loading="busy === 'save'"
              @click="saveSettings"
            >
              保存设置
            </NButton>
            <NButton size="small" quaternary :disabled="!dirty" @click="resetEditing">放弃修改</NButton>
            <span v-if="dirty" class="mono" style="font-size: 11px; color: var(--ux-accent); align-self: center">
              ● 有未保存修改
            </span>
          </div>
        </div>
        <p v-else-if="!showSettings" style="margin: 0; font-size: 12px; color: var(--ux-text-faint)">
          自启 / 崩溃重启 / 激活二进制 / 启动参数（PUT /api/server 全量替换）
        </p>
      </section>
    </template>
  </div>
</template>

<style scoped>
.grid-2 {
  display: grid;
  gap: 12px;
  grid-template-columns: 1fr;
}

@media (min-width: 560px) {
  .grid-2 {
    grid-template-columns: 1fr 1fr;
  }
}

.link-more {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 12px;
  color: var(--ux-text-dim);
  transition: color 0.15s ease;
}

.link-more:hover {
  color: var(--ux-accent);
}
</style>
