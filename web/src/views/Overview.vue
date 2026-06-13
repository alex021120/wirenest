<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { NGrid, NGi, NCard, NText, NSpace, NTag, NButton, NAlert, NPopconfirm } from 'naive-ui'
import { useMessage } from 'naive-ui'
import { api, fmtBytes, type Overview, type SystemInfo } from '../api'
import StatCard from '../components/StatCard.vue'

const message = useMessage()
const data = ref<Overview | null>(null)
const sys = ref<SystemInfo | null>(null)
const loading = ref(false)

// --- live throughput (bytes/sec), derived from deltas of cumulative counters ---
const POLL_MS = 2000
const upSpeed = ref(0)
const downSpeed = ref(0)
let prev: { tx: number; rx: number; t: number } | null = null
let timer: ReturnType<typeof setInterval> | undefined

const fmtSpeed = (n: number) => fmtBytes(n) + '/s'

function sample(ov: Overview) {
  const now = Date.now()
  if (prev) {
    const dt = (now - prev.t) / 1000
    if (dt > 0) {
      // Clamp negatives (counters reset on interface restart).
      upSpeed.value = Math.max(0, (ov.txBytes - prev.tx) / dt)
      downSpeed.value = Math.max(0, (ov.rxBytes - prev.rx) / dt)
    }
  }
  prev = { tx: ov.txBytes, rx: ov.rxBytes, t: now }
}

async function tick() {
  try {
    const ov = await api.overview()
    data.value = ov
    sample(ov)
  } catch {
    /* transient; keep last values */
  }
}

const stateTag = computed(() => {
  if (!data.value) return null
  if (!data.value.configured) return { type: 'error' as const, text: '未配置 · 找不到 wg0.conf' }
  if (!data.value.live) return { type: 'warning' as const, text: '接口未运行 · 仅静态配置' }
  return { type: 'success' as const, text: '运行中' }
})

function fmtUptime(sec: number): string {
  if (!sec) return '—'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d) return `${d} 天 ${h} 小时`
  if (h) return `${h} 小时 ${m} 分`
  return `${m} 分`
}

async function load() {
  loading.value = true
  try {
    const [ov, s] = await Promise.all([api.overview(), api.system()])
    data.value = ov
    sys.value = s
    sample(ov)
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

// --- interface lifecycle control (start / stop / restart) ---
const acting = ref<'' | 'up' | 'down' | 'restart'>('')
async function control(action: 'up' | 'down' | 'restart') {
  acting.value = action
  try {
    await api.interfaceControl(action)
    const label = action === 'up' ? '已启动' : action === 'down' ? '已停止' : '已重启'
    message.success('WireGuard 接口' + label)
    await load()
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    acting.value = ''
  }
}

onMounted(() => {
  load()
  timer = setInterval(tick, POLL_MS)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <n-space vertical :size="22">
    <div class="page-head">
      <div>
        <div class="page-title">总览</div>
        <div class="page-desc">WireGuard 接口实时状态一览</div>
      </div>
      <n-space align="center" :size="10">
        <n-tag v-if="stateTag" round size="small" :bordered="false" :type="stateTag.type">
          {{ stateTag.text }}
        </n-tag>
        <n-button size="small" secondary :loading="loading" @click="load">刷新</n-button>
      </n-space>
    </div>

    <n-grid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" :item-responsive="true">
      <n-gi span="4 s:2 m:1">
        <stat-card :index="0" label="在线客户端" :value="data?.online ?? 0" icon-color="#16A34A" icon-bg="#E9F7EF">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M5 12.55a11 11 0 0 1 14.08 0 M1.42 9a16 16 0 0 1 21.16 0 M8.53 16.11a6 6 0 0 1 6.95 0 M12 20h.01" />
          </svg>
        </stat-card>
      </n-gi>
      <n-gi span="4 s:2 m:1">
        <stat-card :index="1" label="客户端总数" :value="data?.clientsTotal ?? 0" icon-color="#C79A00" icon-bg="#FEF6E0">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2 M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8 M23 21v-2a4 4 0 0 0-3-3.87 M16 3.13a4 4 0 0 1 0 7.75" />
          </svg>
        </stat-card>
      </n-gi>
      <n-gi span="4 s:2 m:1">
        <stat-card :index="2" label="上行网速" :value="upSpeed" :format="fmtSpeed" :animate="false" icon-color="#16A34A" icon-bg="#E9F7EF">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 19V6 M5 12l7-7 7 7" />
          </svg>
        </stat-card>
      </n-gi>
      <n-gi span="4 s:2 m:1">
        <stat-card :index="3" label="下行网速" :value="downSpeed" :format="fmtSpeed" :animate="false" icon-color="#2563EB" icon-bg="#E8F1FE">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 5v13 M19 12l-7 7-7-7" />
          </svg>
        </stat-card>
      </n-gi>
    </n-grid>

    <n-card title="接口信息" :bordered="true">
      <template #header-extra>
        <n-tag v-if="data" size="small" round :bordered="false" :type="data.live ? 'success' : 'warning'">
          {{ data.live ? '运行中' : '未运行' }}
        </n-tag>
      </template>
      <div class="info-grid">
        <div class="info-item">
          <span class="info-k">接口</span>
          <span class="info-v">{{ data?.interface ?? 'wg0' }}</span>
        </div>
        <div class="info-item">
          <span class="info-k">地址</span>
          <span class="info-v">{{ data?.address?.join(', ') || '—' }}</span>
        </div>
        <div class="info-item">
          <span class="info-k">监听端口</span>
          <span class="info-v">{{ data?.listenPort || '—' }}</span>
        </div>
        <div class="info-item">
          <span class="info-k">累计上行</span>
          <span class="info-v">{{ fmtBytes(data?.txBytes ?? 0) }}</span>
        </div>
        <div class="info-item">
          <span class="info-k">累计下行</span>
          <span class="info-v">{{ fmtBytes(data?.rxBytes ?? 0) }}</span>
        </div>
        <div class="info-item">
          <span class="info-k">更新时间</span>
          <span class="info-v">{{ data?.lastUpdated ? new Date(data.lastUpdated).toLocaleString() : '—' }}</span>
        </div>
      </div>

      <div class="iface-actions">
        <span class="iface-actions-label">接口控制</span>
        <div class="iface-actions-btns">
          <n-button
            size="small"
            secondary
            type="success"
            :loading="acting === 'up'"
            :disabled="data?.live || !data?.configured || !!acting"
            @click="control('up')"
          >
            启动
          </n-button>
          <n-popconfirm @positive-click="control('down')">
            <template #trigger>
              <n-button size="small" secondary type="error" :loading="acting === 'down'" :disabled="!data?.live || !!acting">
                停止
              </n-button>
            </template>
            停止后所有客户端将断开连接，确认停止接口？
          </n-popconfirm>
          <n-popconfirm @positive-click="control('restart')">
            <template #trigger>
              <n-button size="small" secondary :loading="acting === 'restart'" :disabled="!data?.configured || !!acting">
                重启
              </n-button>
            </template>
            重启会让已连接的客户端短暂断开，确认重启接口？
          </n-popconfirm>
        </div>
      </div>
    </n-card>

    <n-card title="系统信息" :bordered="true">
      <div class="sys-status">
        <div class="sys-pill">
          <span class="sys-label">WireGuard 接口</span>
          <n-tag size="small" round :bordered="false" :type="sys?.wgRunning ? 'success' : 'warning'">
            {{ sys?.wgRunning ? '运行中' : '未运行' }}
          </n-tag>
        </div>
        <div class="sys-pill">
          <span class="sys-label">IPv4 转发</span>
          <n-tag size="small" round :bordered="false" :type="sys?.ipv4Forwarding ? 'success' : 'error'">
            {{ sys?.ipv4Forwarding ? '已开启' : '未开启' }}
          </n-tag>
        </div>
        <div class="sys-pill">
          <span class="sys-label">内核模块</span>
          <n-tag size="small" round :bordered="false" :type="sys?.wgModuleLoaded ? 'success' : 'warning'">
            {{ sys?.wgModuleLoaded ? '已加载' : '未加载' }}
          </n-tag>
        </div>
      </div>

      <n-alert v-if="sys && !sys.ipv4Forwarding" type="warning" :bordered="false" style="margin: 14px 0">
        异地组网需要开启 IPv4 转发（<n-text code>net.ipv4.ip_forward=1</n-text>），否则客户端之间无法互相访问。
      </n-alert>

      <div class="info-grid">
        <div class="info-item"><span class="info-k">主机名</span><span class="info-v">{{ sys?.hostname || '—' }}</span></div>
        <div class="info-item"><span class="info-k">操作系统</span><span class="info-v">{{ sys?.os || '—' }}</span></div>
        <div class="info-item"><span class="info-k">内核</span><span class="info-v">{{ sys?.kernel || '—' }}</span></div>
        <div class="info-item"><span class="info-k">架构</span><span class="info-v">{{ sys?.arch || '—' }}</span></div>
        <div class="info-item"><span class="info-k">运行时长</span><span class="info-v">{{ fmtUptime(sys?.uptime ?? 0) }}</span></div>
        <div class="info-item"><span class="info-k">CPU</span><span class="info-v">{{ sys?.cpuCount ?? '—' }} 核 · 负载 {{ sys?.load1?.toFixed(2) ?? '—' }}</span></div>
        <div class="info-item"><span class="info-k">内存</span><span class="info-v">{{ sys ? fmtBytes(sys.memUsed) + ' / ' + fmtBytes(sys.memTotal) : '—' }}</span></div>
        <div class="info-item"><span class="info-k">WireGuard 版本</span><span class="info-v">{{ sys?.wgVersion || '—' }}</span></div>
      </div>
    </n-card>
  </n-space>
</template>

<style scoped>
.page-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
}
.page-title {
  font-size: 22px;
  font-weight: 650;
  color: #1f2933;
  letter-spacing: -0.01em;
}
.page-desc {
  font-size: 13px;
  color: #9aa5b1;
  margin-top: 3px;
}
.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 18px 28px;
}
.info-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.info-k {
  font-size: 12px;
  color: #9aa5b1;
}
.info-v {
  font-size: 14px;
  color: #1f2933;
  font-weight: 500;
}
.sys-status {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 28px;
}
.sys-pill {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sys-label {
  font-size: 13px;
  color: #52606d;
}
.iface-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid #f0f2f5;
}
.iface-actions-label {
  font-size: 12px;
  color: #9aa5b1;
}
.iface-actions-btns {
  display: flex;
  gap: 8px;
}
</style>
