<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  NButton,
  NEmpty,
  NIcon,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSpin,
  NSwitch,
  useMessage,
} from 'naive-ui'
import { RefreshOutline } from '@vicons/ionicons5'
import PageHead from '../components/PageHead.vue'
import { getLogs } from '../api'
import { errMsg } from '../utils/format'
import { copyText } from '../utils/copy'

const message = useMessage()

/* ---------------- 参数 ---------------- */

const name = ref<'ctl' | 'server' | 'web'>('ctl')
const tail = ref(200)
const tailOptions = [100, 200, 500].map((n) => ({ label: `${n} 行`, value: n }))

const auto = ref(false)
const AUTO_INTERVAL_MS = 5000

/* ---------------- 加载 ---------------- */

const lines = ref<string[]>([])
const loading = ref(false)
const errorText = ref('')
const fetchedAt = ref(0)
const logView = ref<HTMLDivElement | null>(null)

async function load() {
  loading.value = true
  errorText.value = ''
  try {
    const res = await getLogs(name.value, tail.value)
    lines.value = res.lines ?? []
    fetchedAt.value = Date.now()
    // 日志新行在底部，加载后滚到底
    await nextTick()
    if (logView.value) logView.value.scrollTop = logView.value.scrollHeight
  } catch (e) {
    errorText.value = errMsg(e)
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch([name, tail], () => load())

/* ---------------- 自动刷新（5s） ---------------- */

let autoTimer = 0 as unknown as ReturnType<typeof setInterval>

watch(auto, (v) => {
  clearInterval(autoTimer)
  if (v) autoTimer = setInterval(load, AUTO_INTERVAL_MS)
})

onUnmounted(() => clearInterval(autoTimer))

const fetchedAtText = computed(() => (fetchedAt.value ? new Date(fetchedAt.value).toLocaleTimeString() : '—'))

async function copyAll() {
  const ok = await copyText(lines.value.join('\n'))
  ok ? message.success(`已复制 ${lines.value.length} 行`) : message.error('复制失败')
}
</script>

<template>
  <div class="page">
    <PageHead title="日志" desc="ctl / server / web 三路日志">
      <template #actions>
        <NButton size="small" quaternary circle :loading="loading" @click="load">
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
        </NButton>
      </template>
    </PageHead>

    <!-- 控制条 -->
    <section class="card reveal log-toolbar" style="--i: 1">
      <NRadioGroup v-model:value="name" size="small">
        <NRadioButton value="ctl">ctl</NRadioButton>
        <NRadioButton value="server">server</NRadioButton>
        <NRadioButton value="web">web</NRadioButton>
      </NRadioGroup>

      <div class="toolbar-right">
        <NSelect v-model:value="tail" :options="tailOptions" size="small" style="width: 92px" />
        <label class="auto-toggle">
          <span>自动 {{ auto ? '· 5s' : '' }}</span>
          <NSwitch v-model:value="auto" size="small" />
        </label>
      </div>
    </section>

    <!-- 内容 -->
    <section class="card reveal" style="--i: 2">
      <div class="card-head">
        <span class="sec-label">{{ name }} log · {{ lines.length }} 行</span>
        <span class="mono fetched-at" @click="copyAll" title="点击复制全部">
          {{ fetchedAtText }} {{ lines.length ? '· 复制' : '' }}
        </span>
      </div>

      <div v-if="errorText">
        <p class="error-text">{{ errorText }}</p>
        <NButton size="small" secondary @click="load">重试</NButton>
      </div>

      <div v-else-if="loading && !lines.length" class="page-loading" style="min-height: 100px">
        <NSpin size="small" />
      </div>

      <NEmpty v-else-if="!lines.length" description="暂无日志（文件可能不存在或为空）" size="small" />

      <div v-else ref="logView" class="logview mono">
        <div v-for="(l, i) in lines" :key="i" class="log-line">{{ l }}</div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.log-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  padding: 12px 14px;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.auto-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--ux-text-dim);
  white-space: nowrap;
}

.fetched-at {
  font-size: 11px;
  color: var(--ux-text-faint);
  cursor: pointer;
}

.fetched-at:hover {
  color: var(--ux-accent);
}

.logview {
  max-height: 58vh;
  overflow: auto;
  border-radius: 10px;
  border: 1px solid var(--ux-border);
  background: var(--ux-accent-soft);
  padding: 10px 12px;
  font-size: 11.5px;
  line-height: 1.7;
}

.log-line {
  white-space: pre;
  word-break: normal;
  overflow-x: auto;
  color: var(--ux-text-dim);
}

.log-line:first-child {
  color: var(--ux-text-faint);
}
</style>
