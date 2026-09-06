<script setup lang="ts">
import { computed, onMounted, onScopeDispose, ref, watch } from 'vue'
import {
  NAlert,
  NButton,
  NIcon,
  NPopconfirm,
  NSelect,
  NSpin,
  NTabs,
  NTabPane,
  NTag,
  NUpload,
  NInput,
  useDialog,
  useMessage,
} from 'naive-ui'
import type { UploadFileInfo } from 'naive-ui'
import {
  CheckmarkOutline,
  CloudUploadOutline,
  CopyOutline,
  DownloadOutline,
  RefreshOutline,
  TrashOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import PageHead from '../components/PageHead.vue'
import { binCleanup, binDownload, binImportRest, binList, binRemove, binSources, binUse, getTask } from '../api'
import type { Binary, BinSource, Task } from '../api/types'
import { mode } from '../api/client'
import { useStatusStore } from '../composables/useStatusStore'
import { errMsg, formatDateTime, humanBytes } from '../utils/format'
import { copyText } from '../utils/copy'

const message = useMessage()
const dialog = useDialog()
const { status, refresh } = useStatusStore()
const isKsu = mode === 'ksu'

/* ---------------- 数据 ---------------- */

const servers = ref<Binary[]>([])
const gadgets = ref<Binary[]>([])
const sources = ref<BinSource[]>([])
const loading = ref(false)

onMounted(load)

async function load() {
  loading.value = true
  try {
    const [list, src] = await Promise.all([binList(), status.value ? Promise.resolve(null) : refresh()])
    void src
    servers.value = list.servers ?? []
    gadgets.value = list.gadgets ?? []
    sources.value = await binSources()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}

async function reloadList() {
  const [list] = await Promise.all([binList(), refresh()])
  servers.value = list.servers ?? []
  gadgets.value = list.gadgets ?? []
}

/* ---------------- 分组 / 激活 ---------------- */

const groupTab = ref<'server' | 'gadget'>('server')
const currentList = computed(() => (groupTab.value === 'server' ? servers.value : gadgets.value))
const activeFile = computed(() =>
  groupTab.value === 'server' ? status.value?.settings.server.active : status.value?.settings.gadget.active,
)

const VARIANT_TYPE: Record<string, 'success' | 'warning' | 'error' | 'info' | 'default'> = {
  official: 'success',
  florida: 'warning',
  undetected: 'error',
  custom: 'info',
}

const busyFile = ref('')

async function use(bin: Binary) {
  if (busyFile.value) return
  busyFile.value = bin.file
  try {
    await binUse(groupTab.value, bin.file)
    message.success(`已激活 ${bin.file}`)
    await reloadList()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    busyFile.value = ''
  }
}

async function remove(bin: Binary) {
  if (busyFile.value) return
  busyFile.value = bin.file
  try {
    await binRemove(groupTab.value, bin.file)
    message.success(`已删除 ${bin.file}`)
    await reloadList()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    busyFile.value = ''
  }
}

async function cleanup() {
  try {
    const res = await binCleanup()
    if (!res.removed.length) {
      message.info('没有可清理的未激活文件')
      return
    }
    dialog.success({
      title: `已清理 ${res.removed.length} 个未激活文件`,
      content: res.removed.join('\n'),
      positiveText: '知道了',
    })
    await reloadList()
  } catch (e) {
    message.error(errMsg(e))
  }
}

/* ---------------- task 轮询（下载/导入共用） ---------------- */

const task = ref<Task | null>(null)
const steps = ref<string[]>([])
let pollTimer = 0 as unknown as ReturnType<typeof setInterval>

const PHASE_TEXT: Record<string, string> = {
  resolve: '解析版本',
  download: '下载中',
  decompress: '解压中',
  install: '安装中',
}

function stopPolling() {
  clearInterval(pollTimer)
  pollTimer = 0 as unknown as ReturnType<typeof setInterval>
}

async function pollOnce(id: string) {
  try {
    const t = await getTask(id)
    task.value = t
    if (t.state === 'running' && !steps.value.includes(t.phase)) steps.value.push(t.phase)
    if (t.state === 'done') {
      stopPolling()
      message.success(`安装完成：${t.detail}`)
      task.value = null
      await reloadList()
    } else if (t.state === 'error') {
      stopPolling()
      // 错误留在卡片里展示 t.error
    }
  } catch (e) {
    stopPolling()
    message.error(errMsg(e))
  }
}

function startPolling(id: string) {
  steps.value = []
  task.value = null
  stopPolling()
  void pollOnce(id)
  pollTimer = setInterval(() => pollOnce(id), 900)
}

onScopeDispose(stopPolling)

/* ---------------- 下载 ---------------- */

const dlVariant = ref('official')
const dlType = ref<'server' | 'gadget'>('server')
const dlVersion = ref('')
const downloading = ref(false)

const variantOptions = computed(() => sources.value.map((s) => ({ label: `${s.variant}（${s.versions.length} 个版本）`, value: s.variant })))
const versionOptions = computed(() => {
  const src = sources.value.find((s) => s.variant === dlVariant.value)
  return (src?.versions ?? []).map((v) => ({ label: v, value: v }))
})

// 切源时版本重置为该源最新
watch(dlVariant, () => {
  dlVersion.value = versionOptions.value[0]?.value ?? ''
}, { immediate: true })

async function startDownload() {
  if (!dlVersion.value || downloading.value || task.value) return
  downloading.value = true
  task.value = null
  try {
    const res = await binDownload({ variant: dlVariant.value, type: dlType.value, version: dlVersion.value })
    if ('task_id' in res) {
      startPolling(res.task_id)
    } else {
      // ksu 同步模式直接拿到 Binary
      message.success(`下载完成：${res.file}`)
      await reloadList()
    }
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    downloading.value = false
  }
}

/* ---------------- 导入 ---------------- */

const impType = ref<'server' | 'gadget'>('server')
const impVersion = ref('')
const impFiles = ref<UploadFileInfo[]>([])
const importing = ref(false)

async function startImport() {
  const f = impFiles.value[0]?.file
  if (!f) {
    message.warning('请先选择文件')
    return
  }
  if (!impVersion.value.trim()) {
    message.warning('请填写版本号（如 17.2.14）')
    return
  }
  if (importing.value || task.value) return
  importing.value = true
  try {
    const res = await binImportRest(impType.value, impVersion.value.trim(), f)
    impFiles.value = []
    impVersion.value = ''
    startPolling(res.task_id)
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    importing.value = false
  }
}

/* ---------------- 复制 ---------------- */

async function copy(label: string, text: string) {
  const ok = await copyText(text)
  ok ? message.success(`已复制${label}`) : message.error('复制失败')
}
</script>

<template>
  <div class="page">
    <PageHead title="二进制管理" desc="server / gadget 清单 · 下载 · 导入 · 激活">
      <template #actions>
        <NButton size="small" quaternary circle :loading="loading" @click="load">
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
        </NButton>
      </template>
    </PageHead>

    <!-- 获取二进制：下载 | 导入 -->
    <section class="card reveal fetch-card" style="--i: 1">
      <div class="fetch-col">
        <span class="sec-label">
          <NIcon :size="13" style="vertical-align: -2px"><DownloadOutline /></NIcon>
          下载
        </span>
        <div class="form-grid">
          <div class="form-item">
            <span class="form-label">源</span>
            <NSelect v-model:value="dlVariant" :options="variantOptions" size="small" />
          </div>
          <div class="form-item">
            <span class="form-label">类型</span>
            <NSelect
              v-model:value="dlType"
              :options="[
                { label: 'server（可执行）', value: 'server' },
                { label: 'gadget（.so 注入库）', value: 'gadget' },
              ]"
              size="small"
            />
          </div>
          <div class="form-item" style="grid-column: 1 / -1">
            <span class="form-label">版本（可手动输入）</span>
            <NSelect
              v-model:value="dlVersion"
              :options="versionOptions"
              size="small"
              filterable
              tag
              placeholder="选择或输入版本号"
            />
          </div>
          <NButton
            type="primary"
            size="small"
            block
            :loading="downloading"
            :disabled="!!task"
            @click="startDownload"
          >
            <template #icon><NIcon><DownloadOutline /></NIcon></template>
            {{ isKsu ? '下载（同步执行，需等待）' : '开始下载' }}
          </NButton>
        </div>
      </div>

      <div class="fetch-divider" />

      <div class="fetch-col">
        <span class="sec-label">
          <NIcon :size="13" style="vertical-align: -2px"><CloudUploadOutline /></NIcon>
          导入
        </span>
        <!-- ksu 模式：无文件选择器，给指引 -->
        <NAlert v-if="isKsu" type="info" :bordered="false">
          KSU 模式不支持在线上传。请在 PC 远程模式导入，或 adb push 后用命令行执行
          <span class="mono">utsusemi-ctl</span> 导入。
        </NAlert>
        <div v-else class="form-grid">
          <div class="form-item">
            <span class="form-label">类型</span>
            <NSelect
              v-model:value="impType"
              :options="[
                { label: 'server（可执行）', value: 'server' },
                { label: 'gadget（.so 注入库）', value: 'gadget' },
              ]"
              size="small"
            />
          </div>
          <div class="form-item">
            <span class="form-label">版本</span>
            <NInput v-model:value="impVersion" size="small" placeholder="如 17.2.14" />
          </div>
          <div class="form-item" style="grid-column: 1 / -1">
            <span class="form-label">文件</span>
            <NUpload
              v-model:file-list="impFiles"
              :max="1"
              :default-upload="false"
              directory-dialect
              style="width: 100%"
            >
              <NButton size="small" secondary block>选择文件</NButton>
            </NUpload>
          </div>
          <NButton size="small" block secondary type="primary" :loading="importing" :disabled="!!task" @click="startImport">
            <template #icon><NIcon><CloudUploadOutline /></NIcon></template>
            上传并导入
          </NButton>
        </div>
      </div>
    </section>

    <!-- task 进度 -->
    <section v-if="task" class="card reveal task-card" style="--i: 2">
      <div class="card-head">
        <span class="sec-label">任务进度</span>
        <NTag size="small" :type="task.state === 'error' ? 'error' : task.state === 'done' ? 'success' : 'info'">
          {{ task.state === 'error' ? '失败' : task.state === 'done' ? '完成' : '进行中' }}
        </NTag>
      </div>
      <div class="phase-pills">
        <span v-for="(s, i) in steps" :key="s" class="phase-pill" :class="{ cur: i === steps.length - 1 && task.state === 'running' }">
          {{ PHASE_TEXT[s] ?? s }}
        </span>
        <span v-if="task.state === 'running'" class="phase-pill pending">…</span>
      </div>
      <p class="mono task-detail">{{ task.state === 'error' ? task.error : task.detail }}</p>
      <NAlert v-if="task.state === 'error'" type="error" :bordered="false">{{ task.error }}</NAlert>
    </section>

    <!-- 分组列表 -->
    <section class="card reveal" style="--i: 3; padding-bottom: 8px">
      <div class="card-head">
        <NTabs v-model:value="groupTab" type="segment" size="small" style="flex: 1; min-width: 0">
          <NTabPane name="server" :tab="`server（${servers.length}）`" />
          <NTabPane name="gadget" :tab="`gadget（${gadgets.length}）`" />
        </NTabs>
        <NPopconfirm @positive-click="cleanup">
          <template #trigger>
            <NButton size="tiny" quaternary type="error">清理未激活</NButton>
          </template>
          删除所有未激活的 server / gadget 文件？此操作不可恢复。
        </NPopconfirm>
      </div>

      <div v-if="loading && !currentList.length" class="page-loading" style="min-height: 120px">
        <NSpin size="small" />
      </div>

      <NAlert v-else-if="!currentList.length" type="default" :bordered="false" style="margin-bottom: 12px">
        暂无 {{ groupTab }} 二进制，先在上方下载或导入。
      </NAlert>

      <div
        v-for="b in currentList"
        :key="b.file"
        class="bin-card"
        :class="{ active: b.file === activeFile, missing: b.missing }"
      >
        <div class="bin-head">
          <div class="bin-title">
            <NTag size="small" :type="VARIANT_TYPE[b.variant] ?? 'default'">{{ b.variant }}</NTag>
            <span class="mono bin-version">{{ b.version }}</span>
            <span class="bin-meta mono">{{ b.arch }} · {{ b.elf_type }}</span>
            <NTag v-if="b.file === activeFile" size="small" type="success" :bordered="false">已激活</NTag>
            <NTag v-if="b.missing" size="small" type="error">文件丢失</NTag>
          </div>
        </div>

        <p v-if="b.missing" class="missing-hint">
          <NIcon :size="13" style="vertical-align: -2px"><WarningOutline /></NIcon>
          manifest 有记录但文件已丢失，请重新下载或导入
        </p>

        <div class="kv">
          <span class="kv-key">file</span>
          <span class="kv-val linkish" :title="b.file" @click="copy('文件名', b.file)">{{ b.file }}</span>
        </div>
        <div class="kv">
          <span class="kv-key">sha256</span>
          <span class="kv-val linkish" :title="b.sha256" @click="copy('SHA256', b.sha256)">
            {{ b.sha256 ? b.sha256.slice(0, 12) + '…' : '—' }}
          </span>
        </div>
        <div class="kv">
          <span class="kv-key">大小 / 添加时间</span>
          <span class="kv-val">{{ humanBytes(b.size) }} · {{ formatDateTime(b.added_at) }}</span>
        </div>
        <div v-if="b.source" class="kv">
          <span class="kv-key">source</span>
          <span class="kv-val">
            <a :href="b.source" target="_blank" rel="noreferrer">来源</a>
            <NIcon :size="12" class="copy-ic" @click="copy('来源链接', b.source)"><CopyOutline /></NIcon>
          </span>
        </div>

        <div class="action-row" style="margin-top: 8px">
          <NButton
            size="tiny"
            type="primary"
            secondary
            :disabled="b.missing || b.file === activeFile"
            :loading="busyFile === b.file"
            @click="use(b)"
          >
            <template #icon><NIcon><CheckmarkOutline /></NIcon></template>
            激活
          </NButton>
          <NPopconfirm v-if="b.file !== activeFile" @positive-click="remove(b)">
            <template #trigger>
              <NButton size="tiny" quaternary type="error">
                <template #icon><NIcon><TrashOutline /></NIcon></template>
                删除
              </NButton>
            </template>
            删除 {{ b.file }}？
          </NPopconfirm>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.fetch-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.fetch-divider {
  height: 1px;
  background: var(--ux-border);
}

@media (min-width: 640px) {
  .fetch-card {
    flex-direction: row;
  }
  .fetch-divider {
    width: 1px;
    height: auto;
  }
}

.fetch-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.bin-card {
  border: 1px solid var(--ux-border);
  border-radius: 12px;
  padding: 12px 14px;
  margin-bottom: 10px;
  transition: border-color 0.2s ease;
}

.bin-card.active {
  border-color: var(--ux-accent);
  box-shadow: 0 0 0 1px var(--ux-accent-soft);
}

.bin-card.missing {
  border-color: var(--ux-danger);
}

.bin-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 4px;
}

.bin-title {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.bin-version {
  font-size: 15px;
  font-weight: 700;
}

.bin-meta {
  font-size: 11px;
  color: var(--ux-text-dim);
}

.missing-hint {
  margin: 2px 0 6px;
  font-size: 12px;
  color: var(--ux-danger);
}

.linkish {
  cursor: pointer;
}

.linkish:hover {
  color: var(--ux-accent);
}

.copy-ic {
  cursor: pointer;
  vertical-align: -2px;
  margin-left: 6px;
  color: var(--ux-text-faint);
}

.copy-ic:hover {
  color: var(--ux-accent);
}

.task-card .phase-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 8px;
}

.phase-pill {
  font-size: 11px;
  padding: 3px 10px;
  border-radius: 999px;
  border: 1px solid var(--ux-border);
  color: var(--ux-text-dim);
}

.phase-pill.cur {
  border-color: var(--ux-accent);
  color: var(--ux-accent);
  animation: pillPulse 1.6s ease infinite;
}

.phase-pill.pending {
  border-style: dashed;
}

@keyframes pillPulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 var(--ux-accent-soft);
  }
  50% {
    box-shadow: 0 0 0 4px var(--ux-accent-soft);
  }
}

.task-detail {
  margin: 0 0 4px;
  font-size: 12px;
  color: var(--ux-text-dim);
}
</style>
