<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  NButton,
  NCheckbox,
  NEmpty,
  NIcon,
  NInput,
  NInputNumber,
  NSelect,
  NSpin,
  NSwitch,
  useMessage,
} from 'naive-ui'
import { ChevronDownOutline, RefreshOutline, SearchOutline } from '@vicons/ionicons5'
import PageHead from '../components/PageHead.vue'
import { getApps, setGadget } from '../api'
import type { AppItem, Rule } from '../api/types'
import { useStatusStore } from '../composables/useStatusStore'
import { errMsg } from '../utils/format'

const message = useMessage()
const { status, refresh } = useStatusStore()

/* ---------------- 数据加载 ---------------- */

const apps = ref<AppItem[]>([])
const loading = ref(false)
const loadError = ref('')

/** rules 编辑副本：包名 → Rule；未在应用列表中的既有规则保留（保存时合并） */
const ruleMap = reactive(new Map<string, Rule>())
let initialJson = '[]'
let ruleInited = false

onMounted(load)

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [, ok] = await Promise.all([getApps(), status.value ? Promise.resolve(true) : refresh()])
    void ok
    syncFromStatus()
  } catch (e) {
    loadError.value = errMsg(e)
  } finally {
    loading.value = false
  }
}

// status 到达后初始化 rules 基线（仅首次或放弃/保存后重建）
function syncFromStatus() {
  const rules = status.value?.rules.rules
  if (!rules || loading.value) return
  ruleMap.clear()
  for (const r of rules) ruleMap.set(r.app_name, JSON.parse(JSON.stringify(r)))
  initialJson = serialize()
  ruleInited = true
}

watch(
  () => status.value,
  (s) => {
    if (s && !ruleInited) syncFromStatus()
  },
  { immediate: true },
)

function serialize(): string {
  return JSON.stringify([...ruleMap.values()])
}

/* ---------------- 过滤 / 排序 ---------------- */

const search = ref('')
const showSystem = ref(false)

interface Row {
  pkg: string
  system: boolean
  rule?: Rule
}

const rows = computed<Row[]>(() => {
  const q = search.value.trim().toLowerCase()
  const list = apps.value
    .filter((a) => showSystem.value || !a.system)
    .filter((a) => !q || a.package.toLowerCase().includes(q))
    .map((a) => ({ pkg: a.package, system: a.system, rule: ruleMap.get(a.package) }))
  // 已启用 > 已配置未启用 > 未配置；组内按包名
  const rank = (r: Row) => (r.rule?.enabled ? 0 : r.rule ? 1 : 2)
  return list.sort((a, b) => rank(a) - rank(b) || a.pkg.localeCompare(b.pkg))
})

const enabledCount = computed(() => [...ruleMap.values()].filter((r) => r.enabled).length)

/* ---------------- 展开 / 编辑 ---------------- */

const expanded = ref(new Set<string>())

function toggleExpand(pkg: string) {
  const s = new Set(expanded.value)
  if (s.has(pkg)) s.delete(pkg)
  else s.add(pkg)
  expanded.value = s
}

/** 开启注入时按契约默认值创建规则 */
function ensureRule(pkg: string): Rule {
  let r = ruleMap.get(pkg)
  if (!r) {
    r = {
      app_name: pkg,
      enabled: false,
      start_up_delay_ms: 0,
      child_gating_enabled: false,
      child_gating_mode: 'freeze',
      custom_gadget: '',
    }
    ruleMap.set(pkg, r)
  }
  return r
}

function setEnabled(pkg: string, val: boolean) {
  ensureRule(pkg).enabled = val
  // 首次勾选时自动展开配置区
  if (val && !expanded.value.has(pkg)) toggleExpand(pkg)
}

const MODE_OPTIONS = [
  { label: 'freeze · 冻结', value: 'freeze' },
  { label: 'wait · 等待', value: 'wait' },
  { label: 'kill · 终止', value: 'kill' },
]

const gadgetOptions = computed(() => [
  { label: `全局默认（${status.value?.settings.gadget.active || '未设置'}）`, value: '' },
  ...(status.value?.gadgets ?? [])
    .filter((g) => !g.missing)
    .map((g) => ({ label: g.file, value: g.file })),
])

/* ---------------- 保存（PUT /api/gadget 全量替换） ---------------- */

const dirty = computed(() => ruleInited && serialize() !== initialJson)
const saving = ref(false)

async function save() {
  if (!dirty.value || saving.value) return
  saving.value = true
  try {
    const res = await setGadget({ rules: [...ruleMap.values()] })
    if (res.apply_error) message.warning(`已保存，但 apply 失败：${res.apply_error}`)
    else message.success(`已应用 ${res.applied} 条规则`)
    await refresh()
    syncFromStatus()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}

function discard() {
  syncFromStatus()
  message.info('已放弃修改')
}
</script>

<template>
  <div class="page">
    <PageHead title="应用注入" desc="勾选启用 · 延时 / child gating / 自定义 gadget">
      <template #actions>
        <NButton size="small" quaternary circle :loading="loading" @click="load">
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
        </NButton>
      </template>
    </PageHead>

    <!-- 搜索 / 过滤 -->
    <section class="card reveal toolbar" style="--i: 1">
      <NInput v-model:value="search" placeholder="搜索包名…" size="small" clearable>
        <template #prefix><NIcon :size="14"><SearchOutline /></NIcon></template>
      </NInput>
      <div class="toolbar-right">
        <span class="mono stat-inline">已启用 {{ enabledCount }} / {{ apps.length }}</span>
        <label class="sys-toggle">
          <span>系统应用</span>
          <NSwitch v-model:value="showSystem" size="small" />
        </label>
      </div>
    </section>

    <!-- 加载失败 -->
    <div v-if="loadError" class="card reveal" style="--i: 2">
      <p class="error-text">{{ loadError }}</p>
      <NButton size="small" secondary @click="load">重试</NButton>
    </div>

    <div v-else-if="loading && !apps.length" class="page-loading">
      <NSpin size="large" />
    </div>

    <!-- 空结果 -->
    <div v-else-if="!rows.length" class="card reveal" style="--i: 2">
      <NEmpty description="没有匹配的应用">
        <template #extra>
          <NButton size="tiny" quaternary @click="showSystem = true">显示系统应用</NButton>
        </template>
      </NEmpty>
    </div>

    <!-- 应用列表 -->
    <section v-else class="reveal" style="--i: 2">
      <div
        v-for="row in rows"
        :key="row.pkg"
        class="card app-row"
        :class="{ enabled: row.rule?.enabled }"
      >
        <div class="app-row-head" @click="toggleExpand(row.pkg)">
          <span class="check-wrap" @click.stop>
            <NCheckbox
              :checked="row.rule?.enabled ?? false"
              size="small"
              @update:checked="(v: boolean) => setEnabled(row.pkg, v)"
            />
          </span>
          <div class="app-row-main">
            <span class="mono app-pkg">{{ row.pkg }}</span>
            <span class="app-tags">
              <span v-if="row.system" class="mini-tag">系统</span>
              <span v-if="row.rule" class="mini-tag" :class="{ on: row.rule.enabled }">已配置</span>
              <span v-if="row.rule?.child_gating_enabled" class="mini-tag accent">gating</span>
            </span>
          </div>
          <NIcon class="chevron" :class="{ open: expanded.has(row.pkg) }" :size="16">
            <ChevronDownOutline />
          </NIcon>
        </div>

        <!-- 展开配置 -->
        <div v-if="expanded.has(row.pkg)" class="app-config">
          <template v-if="row.rule">
            <div class="form-grid two">
              <div class="form-item">
                <span class="form-label">启动延时 (ms)</span>
                <NInputNumber
                  v-model:value="row.rule.start_up_delay_ms"
                  size="small"
                  :min="0"
                  :step="100"
                  style="width: 100%"
                />
              </div>
              <div class="form-item">
                <span class="form-label">child gating</span>
                <div class="inline-switch">
                  <NSwitch v-model:value="row.rule.child_gating_enabled" size="small" />
                  <NSelect
                    v-model:value="row.rule.child_gating_mode"
                    :options="MODE_OPTIONS"
                    size="small"
                    :disabled="!row.rule.child_gating_enabled"
                  />
                </div>
              </div>
              <div class="form-item" style="grid-column: 1 / -1">
                <span class="form-label">custom gadget</span>
                <NSelect v-model:value="row.rule.custom_gadget" :options="gadgetOptions" size="small" />
              </div>
            </div>
          </template>
          <template v-else>
            <div class="no-rule">
              <span>尚未配置注入规则</span>
              <NButton size="tiny" secondary @click="setEnabled(row.pkg, true)">启用注入</NButton>
            </div>
          </template>
        </div>
      </div>
    </section>

    <!-- 底部保存条 -->
    <Transition name="page">
      <div v-if="dirty" class="savebar">
        <span class="mono savebar-text">● 已启用 {{ enabledCount }} 项待保存</span>
        <div class="savebar-actions">
          <NButton size="small" quaternary @click="discard">放弃</NButton>
          <NButton size="small" type="primary" :loading="saving" @click="save">保存并应用</NButton>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 14px;
}

.stat-inline {
  font-size: 11px;
  color: var(--ux-text-dim);
}

.sys-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--ux-text-dim);
}

.app-row {
  padding: 0;
  overflow: hidden;
  transition: border-color 0.2s ease;
}

.app-row.enabled {
  border-color: var(--ux-accent);
}

.app-row + .app-row {
  margin-top: 8px;
}

.app-row-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  cursor: pointer;
  user-select: none;
}

.check-wrap {
  display: inline-flex;
  flex-shrink: 0;
}

.app-row-main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.app-pkg {
  font-size: 12.5px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.app-tags {
  display: inline-flex;
  gap: 4px;
}

.mini-tag {
  font-size: 10px;
  line-height: 1;
  padding: 3px 6px;
  border-radius: 4px;
  background: var(--ux-accent-soft);
  color: var(--ux-text-dim);
}

.mini-tag.on,
.mini-tag.accent {
  color: var(--ux-accent);
}

.chevron {
  color: var(--ux-text-faint);
  transition: transform 0.2s ease;
  flex-shrink: 0;
}

.chevron.open {
  transform: rotate(180deg);
}

.app-config {
  border-top: 1px dashed var(--ux-border);
  padding: 14px;
  background: var(--ux-accent-soft);
}

.inline-switch {
  display: flex;
  align-items: center;
  gap: 8px;
}

.no-rule {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  font-size: 12px;
  color: var(--ux-text-dim);
}
</style>
