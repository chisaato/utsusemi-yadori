<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NGrid,
  NGridItem,
  NIcon,
  NInputNumber,
  NSpin,
  NSwitch,
  NTag,
  useMessage,
} from 'naive-ui'
import {
  CheckmarkCircleOutline,
  CloseCircleOutline,
  CopyOutline,
  FlashOutline,
  HardwareChipOutline,
  RefreshOutline,
  SaveOutline,
  WifiOutline,
} from '@vicons/ionicons5'
import PageHead from '../components/PageHead.vue'
import {
  applyAdbSettings,
  getAdbStatus,
  restartAdb,
  setAdbSettings,
  setAdbTcpip,
  setAdbUsb,
} from '../api'
import type { AdbCurrentStatus, AdbSettings } from '../api/types'
import { errMsg } from '../utils/format'
import { copyText } from '../utils/copy'

const message = useMessage()

/* ---------------- 状态与加载 ---------------- */

const loading = ref(false)
const errorText = ref('')
const current = ref<AdbCurrentStatus>({
  adbd_running: false,
  usb_enabled: false,
  tcpip_enabled: false,
  tcpip_port: -1,
  ips: [],
})

const savedSettings = ref<AdbSettings>({
  usb_enabled: true,
  tcpip_enabled: false,
  port: 5555,
  apply_on_boot: false,
})

// 表单草稿状态
const formDraft = ref<AdbSettings>({
  usb_enabled: true,
  tcpip_enabled: false,
  port: 5555,
  apply_on_boot: false,
})

const isDirty = computed(() => {
  return (
    formDraft.value.usb_enabled !== savedSettings.value.usb_enabled ||
    formDraft.value.tcpip_enabled !== savedSettings.value.tcpip_enabled ||
    formDraft.value.port !== savedSettings.value.port ||
    formDraft.value.apply_on_boot !== savedSettings.value.apply_on_boot
  )
})

async function load() {
  loading.value = true
  errorText.value = ''
  try {
    const res = await getAdbStatus()
    current.value = res.current
    savedSettings.value = { ...res.settings }
    formDraft.value = { ...res.settings }
  } catch (e) {
    errorText.value = errMsg(e)
  } finally {
    loading.value = false
  }
}

onMounted(load)

/* ---------------- 实时动作 ---------------- */

const actionBusy = ref<string | null>(null)

async function toggleUsb(val: boolean) {
  if (actionBusy.value) {
    return
  }
  actionBusy.value = 'usb'
  try {
    const res = await setAdbUsb(val, false)
    current.value = { ...current.value, ...res }
    message.success(val ? '已开启 USB 调试' : '已关闭 USB 调试')
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    actionBusy.value = null
  }
}

async function toggleTcpip(val: boolean) {
  if (actionBusy.value) {
    return
  }
  actionBusy.value = 'tcpip'
  try {
    const port = formDraft.value.port > 0 ? formDraft.value.port : 5555
    const res = await setAdbTcpip(val, port, false)
    current.value = { ...current.value, ...res }
    message.success(val ? `已开启无线调试（端口 ${port}）` : '已关闭无线调试')
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    actionBusy.value = null
  }
}

async function onRestart() {
  if (actionBusy.value) {
    return
  }
  actionBusy.value = 'restart'
  try {
    await restartAdb()
    message.success('已发送 adbd 重启指令')
    await load()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    actionBusy.value = null
  }
}

/* ---------------- 保存与应用配置 ---------------- */

const saveBusy = ref(false)

async function saveConfig(applyNow = false) {
  if (saveBusy.value) {
    return
  }
  saveBusy.value = true
  try {
    const payload = {
      ...formDraft.value,
      apply: applyNow,
    }
    const res = await setAdbSettings(payload)
    savedSettings.value = { ...res.settings }
    formDraft.value = { ...res.settings }
    if (applyNow) {
      message.success('配置已保存并立即生效')
      await load()
    } else {
      message.success('配置已保存')
    }
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    saveBusy.value = false
  }
}

async function onApplySaved() {
  if (actionBusy.value) {
    return
  }
  actionBusy.value = 'apply'
  try {
    const res = await applyAdbSettings()
    current.value = res.current
    savedSettings.value = { ...res.settings }
    formDraft.value = { ...res.settings }
    message.success('已应用已保存的 ADB 配置')
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    actionBusy.value = null
  }
}

function resetForm() {
  formDraft.value = { ...savedSettings.value }
}

async function copyConnectCmd(ip: string) {
  const port = current.value.tcpip_port > 0 ? current.value.tcpip_port : 5555
  const cmd = `adb connect ${ip}:${port}`
  const ok = await copyText(cmd)
  if (ok) {
    message.success(`已复制: ${cmd}`)
  } else {
    message.error('复制失败，请手动选择复制')
  }
}
</script>

<template>
  <div class="page adb-page">
    <PageHead
      title="ADB 调试管理"
      sub="快速开关系统 USB 调试与无线网络调试 (TCP/IP)，持久化保存配置并支持开机自动恢复"
    >
      <template #extra>
        <n-button quaternary size="small" :loading="loading" @click="load">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          刷新
        </n-button>
      </template>
    </PageHead>

    <n-spin :show="loading">
      <div class="content-body">
        <n-alert v-if="errorText" type="error" closable class="mb-4" @close="errorText = ''">
          {{ errorText }}
        </n-alert>

        <!-- 实时控制面板 -->
        <n-grid cols="1 s:2 m:3" :x-gap="16" :y-gap="16" responsive="screen" class="mb-4">
          <!-- adbd 状态 -->
          <n-grid-item>
            <n-card size="small" class="metric-card" :bordered="true">
              <div class="metric-head">
                <span class="label">adbd 守护进程</span>
                <n-tag v-if="current.adbd_running" size="small" type="success" round>
                  <template #icon><n-icon :component="CheckmarkCircleOutline" /></template>
                  运行中
                </n-tag>
                <n-tag v-else size="small" type="default" round>
                  <template #icon><n-icon :component="CloseCircleOutline" /></template>
                  已停止
                </n-tag>
              </div>
              <div class="metric-action mt-3">
                <n-button
                  size="small"
                  secondary
                  :loading="actionBusy === 'restart'"
                  @click="onRestart"
                >
                  <template #icon><n-icon :component="RefreshOutline" /></template>
                  重启 adbd
                </n-button>
              </div>
            </n-card>
          </n-grid-item>

          <!-- USB 调试快速开关 -->
          <n-grid-item>
            <n-card size="small" class="metric-card" :bordered="true">
              <div class="metric-head">
                <span class="label">USB 调试</span>
                <n-switch
                  :value="current.usb_enabled"
                  :loading="actionBusy === 'usb'"
                  @update:value="toggleUsb"
                />
              </div>
              <div class="metric-sub mt-2">
                {{ current.usb_enabled ? '已开启 (通过 USB 线缆调试)' : '已关闭' }}
              </div>
            </n-card>
          </n-grid-item>

          <!-- 无线调试快速开关 -->
          <n-grid-item>
            <n-card size="small" class="metric-card" :bordered="true">
              <div class="metric-head">
                <span class="label">无线网络调试</span>
                <n-switch
                  :value="current.tcpip_enabled"
                  :loading="actionBusy === 'tcpip'"
                  @update:value="toggleTcpip"
                />
              </div>
              <div class="metric-sub mt-2">
                <span v-if="current.tcpip_enabled">
                  监听端口: <strong class="mono">{{ current.tcpip_port }}</strong>
                </span>
                <span v-else>已关闭</span>
              </div>
            </n-card>
          </n-grid-item>
        </n-grid>

        <!-- 无线连接助手卡片（开启无线调试时显示） -->
        <n-card
          v-if="current.tcpip_enabled"
          title="局域网连接助手"
          size="small"
          class="mb-4 connection-card"
          :bordered="true"
        >
          <template #header-extra>
            <n-icon :component="WifiOutline" :size="18" />
          </template>
          <div v-if="current.ips.length === 0" class="muted-text">
            未检测到局域网 IP，请确认设备已连接 Wi-Fi 或以太网。
          </div>
          <div v-else class="ip-list">
            <div v-for="ip in current.ips" :key="ip" class="ip-item">
              <span class="mono ip-text">adb connect {{ ip }}:{{ current.tcpip_port }}</span>
              <n-button size="tiny" quaternary @click="copyConnectCmd(ip)">
                <template #icon><n-icon :component="CopyOutline" /></template>
                复制
              </n-button>
            </div>
          </div>
        </n-card>

        <!-- 持久化配置编辑面板 -->
        <n-card title="ADB 默认偏好设置" size="small" class="settings-card" :bordered="true">
          <template #header-extra>
            <n-tag v-if="isDirty" type="warning" size="small" round>未保存修改</n-tag>
          </template>

          <p class="desc-text">
            在此处自定义默认开启状态与端口号。开启「开机自动应用」后，每次设备启动将自动生效此配置。
          </p>

          <n-form label-placement="left" label-width="160" class="mt-4">
            <n-form-item label="默认启用 USB 调试">
              <n-switch v-model:value="formDraft.usb_enabled" />
            </n-form-item>

            <n-form-item label="默认启用网络调试">
              <n-switch v-model:value="formDraft.tcpip_enabled" />
            </n-form-item>

            <n-form-item label="网络调试端口">
              <n-input-number
                v-model:value="formDraft.port"
                :min="1"
                :max="65535"
                placeholder="默认 5555"
                style="width: 180px"
              />
            </n-form-item>

            <n-form-item label="开机自动应用">
              <div class="d-flex align-center">
                <n-switch v-model:value="formDraft.apply_on_boot" />
                <span class="sub-tip ml-2">系统启动进入 boot 阶段时自动配置 adb</span>
              </div>
            </n-form-item>
          </n-form>

          <div class="actions-row mt-4">
            <n-button
              type="primary"
              :loading="saveBusy"
              :disabled="!isDirty"
              @click="saveConfig(false)"
            >
              <template #icon><n-icon :component="SaveOutline" /></template>
              仅保存配置
            </n-button>

            <n-button
              type="info"
              secondary
              :loading="saveBusy"
              @click="saveConfig(true)"
            >
              <template #icon><n-icon :component="FlashOutline" /></template>
              保存并立即应用
            </n-button>

            <n-button
              quaternary
              :loading="actionBusy === 'apply'"
              @click="onApplySaved"
            >
              <template #icon><n-icon :component="HardwareChipOutline" /></template>
              应用已存配置
            </n-button>

            <n-button
              v-if="isDirty"
              quaternary
              @click="resetForm"
            >
              还原修改
            </n-button>
          </div>
        </n-card>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.adb-page {
  padding-bottom: 2rem;
}

.metric-card {
  border-radius: 8px;
}

.metric-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.metric-head .label {
  font-weight: 500;
  font-size: 14px;
}

.metric-sub {
  font-size: 13px;
  color: var(--n-text-color-3);
}

.connection-card {
  border-radius: 8px;
}

.ip-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ip-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--n-color-embedded);
  padding: 8px 12px;
  border-radius: 6px;
}

.ip-text {
  font-size: 13px;
}

.settings-card {
  border-radius: 8px;
}

.desc-text {
  color: var(--n-text-color-3);
  font-size: 13px;
  margin: 0;
}

.sub-tip {
  font-size: 12px;
  color: var(--n-text-color-3);
}

.actions-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}

.d-flex {
  display: flex;
}

.align-center {
  align-items: center;
}

.ml-2 {
  margin-left: 8px;
}

.mt-2 {
  margin-top: 8px;
}

.mt-3 {
  margin-top: 12px;
}

.mt-4 {
  margin-top: 16px;
}

.mb-4 {
  margin-bottom: 16px;
}
</style>
