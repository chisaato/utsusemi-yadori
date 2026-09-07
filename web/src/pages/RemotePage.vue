<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import {
  NAlert,
  NButton,
  NIcon,
  NInput,
  NInputNumber,
  NSpin,
  useDialog,
  useMessage,
} from 'naive-ui'
import QRCode from 'qrcode'
import {
  CopyOutline,
  EyeOffOutline,
  EyeOutline,
  PowerOutline,
  RefreshOutline,
  ShieldCheckmarkOutline,
} from '@vicons/ionicons5'
import PageHead from '../components/PageHead.vue'
import { setWebToken, webInfo, webStop } from '../api'
import { execCtlRaw, mode } from '../api/client'
import type { WebInfo } from '../api/types'
import { errMsg } from '../utils/format'
import { copyText } from '../utils/copy'

const message = useMessage()
const dialog = useDialog()
const isKsu = mode === 'ksu'

/* ---------------- WebInfo ---------------- */

const info = ref<WebInfo | null>(null)
const loading = ref(false)
const errorText = ref('')

onMounted(load)

async function load() {
  loading.value = true
  errorText.value = ''
  try {
    info.value = await webInfo()
  } catch (e) {
    errorText.value = errMsg(e)
  } finally {
    loading.value = false
  }
}

/* ---------------- 启停 ---------------- */

const busy = ref<'start' | 'stop' | null>(null)
const portEdit = ref(23333)

watch(
  info,
  (v) => {
    if (v?.port) portEdit.value = v.port
  },
  { immediate: true },
)

/** ksu 模式：utsusemi-ctl web start --port N（原生命令，非 api 信封） */
async function ksuStart() {
  if (busy.value) return
  busy.value = 'start'
  try {
    await execCtlRaw(`web start --port ${portEdit.value}`)
    message.success(`远程服务已启动（端口 ${portEdit.value}）`)
    await load()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    busy.value = null
  }
}

/** ksu 模式：utsusemi-ctl web stop */
async function ksuStop() {
  if (busy.value) return
  busy.value = 'stop'
  try {
    await execCtlRaw('web stop')
    message.success('远程服务已停止')
    await load()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    busy.value = null
  }
}

/** REST 模式：POST /api/web/stop（关停后本页面将无法访问，需在设备端重启） */
function restStop() {
  dialog.warning({
    title: '关闭远程服务',
    content: '关闭后本页面将无法访问，需在设备端（KSU 管理器或命令行）重新启动。确认关闭？',
    positiveText: '关闭服务',
    negativeText: '取消',
    onPositiveClick: async () => {
      busy.value = 'stop'
      try {
        await webStop()
        message.success('服务已关闭，即将失去连接')
        await load()
      } catch (e) {
        message.error(errMsg(e))
      } finally {
        busy.value = null
      }
    },
  })
}

/* ---------------- 访问令牌管理（契约 §4.8） ---------------- */

// 后端校验：trim 后 8–128 字符、ASCII 可打印（0x21–0x7E）且不含空白 —— 前端同规则镜像
const TOKEN_RE = /^[\x21-\x7E]{8,128}$/

const showToken = ref(false)
const tokenInput = ref('')
const tokenBusy = ref(false)
/** restart_required=true：新令牌待 web 服务重启生效 */
const needRestart = ref(false)
const restarting = ref(false)

const tokenValid = computed(() => TOKEN_RE.test(tokenInput.value.trim()))
// 与当前令牌相同或未填写时不允许保存
const canSaveToken = computed(() => tokenValid.value && tokenInput.value.trim() !== '' && tokenInput.value.trim() !== info.value?.token)

/** 打码展示：头 4 + 尾 4，中间以 • 遮盖 */
function maskToken(t: string): string {
  if (!t) return '—'
  if (t.length <= 8) return '•'.repeat(t.length)
  return t.slice(0, 4) + '•'.repeat(Math.min(16, t.length - 8)) + t.slice(-4)
}

/** 保存令牌：按传输模式走 request() 双通道（REST PUT /api/web/token | ksu api web/token） */
async function saveToken(payload: { token?: string; generate?: boolean }) {
  if (tokenBusy.value) return
  tokenBusy.value = true
  try {
    const res = await setWebToken(payload)
    message.success('令牌已更新')
    // 重启前旧令牌继续服务当前会话；新令牌用于重启后的新入口链接
    needRestart.value = res.restart_required
    if (res.restart_required) message.warning('新令牌将于 web 服务重启后生效')
    tokenInput.value = ''
    await load()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    tokenBusy.value = false
  }
}

async function saveCustomToken() {
  if (!canSaveToken.value) return
  await saveToken({ token: tokenInput.value.trim() })
}

async function generateToken() {
  await saveToken({ generate: true })
}

/** ksu 快捷重启：web stop → web start --port N，让新令牌立即生效 */
async function restartWeb() {
  if (restarting.value) return
  restarting.value = true
  try {
    await execCtlRaw('web stop')
    await execCtlRaw(`web start --port ${info.value?.port ?? portEdit.value}`)
    message.success('web 服务已重启，新令牌已生效')
    needRestart.value = false
    await load()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    restarting.value = false
  }
}

/* ---------------- 入口二维码 ---------------- */

const qrCanvas = ref<HTMLCanvasElement | null>(null)

const isHttp = /^https?:$/.test(location.protocol)

const entryUrl = computed(() =>
  info.value && isHttp
    ? `${location.origin}${location.pathname}?token=${encodeURIComponent(info.value.token)}`
    : '',
)

watch(
  entryUrl,
  async (u) => {
    if (!u) return
    await nextTick()
    if (qrCanvas.value) {
      await QRCode.toCanvas(qrCanvas.value, u, {
        width: 232,
        margin: 2,
        color: { dark: '#0c1211', light: '#ffffff' },
      })
    }
  },
  { immediate: true },
)

/* ---------------- adb 指引 / 复制 ---------------- */

const port = computed(() => info.value?.port ?? portEdit.value)
const token = computed(() => info.value?.token ?? '')
const adbCmd = computed(() => `adb forward tcp:${port.value} tcp:${port.value}`)
const localUrl = computed(() => `http://127.0.0.1:${port.value}/?token=${token.value}`)

async function copy(label: string, text: string) {
  const ok = await copyText(text)
  ok ? message.success(`已复制${label}`) : message.error('复制失败')
}
</script>

<template>
  <div class="page">
    <PageHead title="远程协同" desc="WebInfo · 入口二维码 · adb 端口转发">
      <template #actions>
        <NButton size="small" quaternary circle :loading="loading" @click="load">
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
        </NButton>
      </template>
    </PageHead>

    <!-- 常驻安全警示（契约 §6） -->
    <NAlert type="warning" :bordered="false" class="reveal" style="--i: 1" title="安全警告">
      远程服务默认监听 <span class="mono">0.0.0.0</span>，同一局域网内任何人都可以访问此面板。
      请仅在可信网络开启，用完及时关停，并妥善保管 token。
    </NAlert>

    <!-- 加载失败 -->
    <div v-if="errorText" class="card reveal" style="--i: 2">
      <p class="error-text">{{ errorText }}</p>
      <NButton size="small" secondary @click="load">重试</NButton>
    </div>

    <div v-else-if="!info" class="page-loading">
      <NSpin size="large" />
    </div>

    <template v-else>
      <!-- WebInfo -->
      <section class="card reveal" style="--i: 2">
        <div class="card-head">
          <span class="sec-label">web 服务</span>
          <span class="state-badge" :class="info.running ? 'on' : 'off'">
            <i class="dot" />
            {{ info.running ? '运行中' : '未运行' }}
          </span>
        </div>
        <div class="kv">
          <span class="kv-key">端口</span>
          <span class="kv-val">{{ info.port }}</span>
        </div>
        <div class="kv">
          <span class="kv-key">访问令牌</span>
          <span class="kv-val linkish" @click="copy('令牌', info.token)">
            {{ info.token || '—' }}
            <NIcon :size="12" style="vertical-align: -2px"><CopyOutline /></NIcon>
          </span>
        </div>
        <div class="kv">
          <span class="kv-key">开机自启</span>
          <span class="kv-val">{{ info.enabled ? '已启用' : '未启用' }}</span>
        </div>

        <div class="action-row">
          <!-- ksu 模式：原生 web start / stop -->
          <template v-if="isKsu">
            <span class="port-field">
              <span class="form-label">端口</span>
              <NInputNumber v-model:value="portEdit" size="small" :min="1" :max="65535" style="width: 110px" />
            </span>
            <NButton type="primary" size="small" :loading="busy === 'start'" @click="ksuStart">
              <template #icon><NIcon><PowerOutline /></NIcon></template>
              {{ info.running ? '重启服务' : '启动服务' }}
            </NButton>
            <NButton v-if="info.running" size="small" secondary type="error" :loading="busy === 'stop'" @click="ksuStop">
              停止服务
            </NButton>
          </template>
          <!-- REST 模式：只读 + 关停 -->
          <template v-else>
            <NButton size="small" type="error" secondary :loading="busy === 'stop'" @click="restStop">
              关闭远程服务
            </NButton>
            <span class="hint-inline">关闭后需在设备端重新启动</span>
          </template>
        </div>
      </section>

      <!-- 访问令牌管理（契约 §4.8） -->
      <section class="card reveal" style="--i: 3">
        <div class="card-head">
          <span class="sec-label">访问令牌</span>
          <NButton text size="tiny" @click="showToken = !showToken">
            <template #icon>
              <NIcon :size="15"><component :is="showToken ? EyeOffOutline : EyeOutline" /></NIcon>
            </template>
            {{ showToken ? '隐藏' : '显示' }}
          </NButton>
        </div>

        <div class="kv">
          <span class="kv-key">当前令牌</span>
          <span class="kv-val mono" :class="{ masked: !showToken }">
            {{ showToken ? info.token : maskToken(info.token) }}
          </span>
        </div>
        <div class="action-row" style="margin: 2px 0 10px">
          <NButton size="tiny" secondary @click="copy('令牌', info.token)">
            <template #icon><NIcon><CopyOutline /></NIcon></template>
            复制令牌
          </NButton>
        </div>

        <div class="form-item">
          <span class="form-label">自定义新令牌 · 8–128 位，ASCII 可打印且不含空格</span>
          <div class="token-row">
            <NInput
              v-model:value="tokenInput"
              class="mono"
              size="small"
              placeholder="留空表示不修改"
              clearable
              :status="tokenInput && !tokenValid ? 'error' : undefined"
              @keydown.enter="saveCustomToken"
            />
            <NButton size="small" secondary :loading="tokenBusy" @click="generateToken">一键生成</NButton>
            <NButton size="small" type="primary" :disabled="!canSaveToken" :loading="tokenBusy" @click="saveCustomToken">
              保存
            </NButton>
          </div>
          <p v-if="tokenInput && !tokenValid" class="field-error">
            长度需 8–128 字符，仅限 ASCII 可打印字符且不含空白
          </p>
        </div>

        <!-- restart_required：提示生效时机 + ksu 快捷重启 -->
        <NAlert v-if="needRestart" type="warning" :bordered="false" style="margin-top: 10px">
          <span class="restart-text">
            新令牌将于 web 服务重启后生效。新令牌用于新的远程入口链接（?token=…）；生效前当前页面会话不受影响。
          </span>
          <div v-if="isKsu" class="action-row" style="margin-top: 8px">
            <NButton size="tiny" type="warning" :loading="restarting" @click="restartWeb">立即重启 web 服务</NButton>
          </div>
          <span v-else class="restart-text">请在设备端重启 web 服务后使用新令牌。</span>
        </NAlert>

        <p class="step-note">
          说明：修改后的令牌用于拼装新的远程入口链接（<span class="mono">?token=…</span>）；
          二维码与 adb 指引会在 web 服务重启后自动使用新令牌。
        </p>
      </section>

      <!-- 入口二维码（仅 http 远程模式可生成有意义的 URL） -->
      <section class="card reveal qr-card" style="--i: 4">
        <div class="card-head">
          <span class="sec-label">入口二维码</span>
        </div>
        <div v-if="entryUrl" class="qr-wrap">
          <canvas ref="qrCanvas" class="qr-canvas" />
          <p class="qr-caption">用手机 / 其他设备扫码直接打开此面板</p>
          <p class="mono qr-url" @click="copy('入口链接', entryUrl)">{{ entryUrl }}</p>
        </div>
        <NAlert v-else type="info" :bordered="false">
          当前在 KSU 管理器内置 WebView 中运行（非 http 来源），无法生成二维码。
          请先启动远程服务，再通过下方 adb forward 或局域网 IP 在浏览器中打开。
        </NAlert>
      </section>

      <!-- adb forward 指引 -->
      <section class="card reveal" style="--i: 5">
        <div class="card-head">
          <span class="sec-label">adb 端口转发</span>
          <NIcon style="color: var(--ux-text-faint)"><ShieldCheckmarkOutline /></NIcon>
        </div>
        <p class="step-text">1. USB 连接设备后在电脑执行：</p>
        <div class="codeblock mono" @click="copy('adb 命令', adbCmd)">{{ adbCmd }}</div>
        <p class="step-text">2. 电脑浏览器打开：</p>
        <div class="codeblock mono" @click="copy('本地地址', localUrl)">{{ localUrl }}</div>
        <p class="step-note">提示：手机与电脑在同一 Wi-Fi 时，也可直接访问 <span class="mono">http://&lt;手机IP&gt;:{{ port }}/?token={{ token }}</span>（手机 IP 见 WLAN 设置）。</p>
      </section>
    </template>
  </div>
</template>

<style scoped>
.reveal :deep(.n-alert__content) {
  font-size: 13px;
}

.port-field {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.hint-inline {
  font-size: 11px;
  color: var(--ux-text-faint);
  align-self: center;
}

/* 令牌操作行：输入 + 生成 + 保存 */
.token-row {
  display: flex;
  gap: 6px;
}

.token-row .n-input {
  flex: 1;
  min-width: 0;
}

.field-error {
  margin: 6px 0 0;
  font-size: 11.5px;
  color: var(--ux-danger);
}

.masked {
  letter-spacing: 0.08em;
  user-select: none;
}

.restart-text {
  font-size: 12.5px;
  line-height: 1.6;
}

.qr-card {
  text-align: center;
}

.qr-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}

.qr-canvas {
  width: 232px;
  height: 232px;
  border-radius: 12px;
  border: 1px solid var(--ux-border);
}

.qr-caption {
  margin: 0;
  font-size: 12px;
  color: var(--ux-text-dim);
}

.qr-url {
  margin: 0;
  font-size: 11px;
  color: var(--ux-text-faint);
  word-break: break-all;
  cursor: pointer;
}

.qr-url:hover {
  color: var(--ux-accent);
}

.step-text {
  margin: 10px 0 6px;
  font-size: 12.5px;
  color: var(--ux-text-dim);
}

.codeblock {
  position: relative;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--ux-accent-soft);
  border: 1px solid var(--ux-border);
  font-size: 12px;
  word-break: break-all;
  cursor: pointer;
  transition: border-color 0.15s ease;
}

.codeblock:hover {
  border-color: var(--ux-accent);
}

.step-note {
  margin: 12px 0 0;
  font-size: 11.5px;
  color: var(--ux-text-faint);
  line-height: 1.6;
}
</style>
