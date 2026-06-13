<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter, RouterView } from 'vue-router'
import { NLayout, NLayoutHeader, NLayoutSider, NLayoutContent, NDropdown, NAvatar, NText, useMessage, useNotification } from 'naive-ui'
import { api, type Overview } from '../api'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const notification = useNotification()

interface NavItem {
  key: string
  label: string
  icon: string
}
const nav: NavItem[] = [
  { key: 'overview', label: '总览', icon: 'M3 12l2-2 4 4 6-6 4 4 2-2 M3 12v7a1 1 0 0 0 1 1h16a1 1 0 0 0 1-1v-7' },
  { key: 'clients', label: '客户端', icon: 'M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2 M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8 M23 21v-2a4 4 0 0 0-3-3.87 M16 3.13a4 4 0 0 1 0 7.75' },
  { key: 'settings', label: '设置', icon: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1.08-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1.08 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z' },
]
const activeKey = computed(() => route.name as string)

function go(key: string) {
  if (key !== activeKey.value) router.push({ name: key })
}

// Live interface status for the sidebar footer.
const overview = ref<Overview | null>(null)
const status = computed(() => {
  if (!overview.value) return { dot: '#cbd2da', text: '加载中…', live: false }
  if (!overview.value.configured) return { dot: '#e5484d', text: '未配置', live: false }
  if (!overview.value.live) return { dot: '#f5a623', text: '接口未运行', live: false }
  return { dot: '#16a34a', text: '运行中', live: true }
})

const userOptions = [{ label: '退出登录', key: 'logout' }]
async function onUser(key: string) {
  if (key === 'logout') {
    try {
      await api.logout()
    } finally {
      message.success('已退出')
      router.push({ name: 'login' })
    }
  }
}

// --- "client came online" notifications (top-right) ---
// Poll the client list; when any peer flips offline→online, pop a toast.
// Lives in the layout so it fires on every page, not just the overview.
const CLIENT_POLL_MS = 5000
const onlineState = new Map<string, boolean>()
let seeded = false
let clientsTimer: ReturnType<typeof setInterval> | undefined

async function pollClients() {
  if (document.hidden) return // pause online-detection polling in a hidden tab
  let res
  try {
    res = await api.clients()
  } catch {
    return // transient; keep last known state
  }
  if (!res.live) return // interface down: handshakes/online are meaningless
  for (const c of res.clients) {
    const was = onlineState.get(c.publicKey)
    // Only notify on a real offline→online flip; first sighting just seeds
    // the baseline (covers initial load and freshly-created clients).
    if (seeded && was === false && c.online) {
      const name = c.name || '未命名'
      // Compact single-line toast: a small pulsing green dot + "「name」已上线".
      notification.create({
        closable: false,
        duration: 3000,
        keepAliveOnHover: true,
        content: () =>
          h('div', { style: 'display:flex;align-items:center;gap:11px;white-space:nowrap;' }, [
            h('span', { class: 'online-dot' }),
            h('span', { style: 'font-size:14.5px;color:#1f2933;' }, [
              h('strong', { style: 'font-weight:600;' }, name),
              ' 已上线',
            ]),
          ]),
      })
    }
    onlineState.set(c.publicKey, c.online)
  }
  // Drop state for clients that no longer exist.
  const present = new Set(res.clients.map((c) => c.publicKey))
  for (const k of [...onlineState.keys()]) {
    if (!present.has(k)) onlineState.delete(k)
  }
  seeded = true
}

onMounted(async () => {
  try {
    overview.value = await api.overview()
  } catch {
    /* footer just shows a neutral state */
  }
  pollClients()
  clientsTimer = setInterval(pollClients, CLIENT_POLL_MS)
})
onUnmounted(() => clearInterval(clientsTimer))
</script>

<template>
  <div class="app-bg" aria-hidden="true">
    <span class="blob b-mint" />
    <span class="blob b-blue" />
    <span class="blob b-amber" />
  </div>
  <n-layout class="app-layout" style="height: 100vh">
    <n-layout-header class="topbar" bordered>
      <div class="brand">
        <div class="brand-mark">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="#1F2933" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 2L3 7v5c0 5 3.8 8.4 9 10 5.2-1.6 9-5 9-10V7l-9-5z" />
          </svg>
        </div>
        <div class="brand-text">
          <span class="brand-name">WireNest</span>
          <span class="brand-sub">WireGuard 面板</span>
        </div>
      </div>

      <n-dropdown :options="userOptions" @select="onUser" trigger="click">
        <div class="user">
          <n-avatar round size="small" style="background: #F2C200; color: #1F2933; font-weight: 600">A</n-avatar>
          <n-text>admin</n-text>
        </div>
      </n-dropdown>
    </n-layout-header>

    <n-layout has-sider style="height: calc(100vh - 60px)">
      <n-layout-sider bordered :width="252" :native-scrollbar="false">
        <div class="sider">
          <div class="nav-label">导航</div>
          <nav class="nav">
            <button
              v-for="item in nav"
              :key="item.key"
              class="nav-item"
              :class="{ active: activeKey === item.key }"
              @click="go(item.key)"
            >
              <span class="nav-indicator" />
              <svg class="nav-icon" viewBox="0 0 24 24" width="19" height="19" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path :d="item.icon" />
              </svg>
              <span class="nav-text">{{ item.label }}</span>
            </button>
          </nav>

          <div class="sider-footer">
            <div class="status-card">
              <span class="status-dot" :class="{ pulse: status.live }" :style="{ background: status.dot }" />
              <div class="status-body">
                <span class="status-iface">{{ overview?.interface ?? 'wg0' }}</span>
                <span class="status-text">{{ status.text }}</span>
              </div>
            </div>
          </div>
        </div>
      </n-layout-sider>

      <n-layout-content content-style="padding: 30px 36px;" :native-scrollbar="false">
        <div class="page">
          <router-view v-slot="{ Component }">
            <transition name="page" mode="out-in">
              <component :is="Component" />
            </transition>
          </router-view>
        </div>
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<style scoped>
/* Fixed gradient + soft blobs behind the whole app; the white sidebar/header
   sit on top, so the gradient shows through the (transparent) content area. */
.app-bg {
  position: fixed;
  inset: 0;
  z-index: 0;
  overflow: hidden;
  pointer-events: none;
  background: linear-gradient(160deg, #fbfcfe 0%, #f1f6fb 100%);
}
.blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
}
.b-mint {
  width: 460px;
  height: 460px;
  background: #b6f0d8; /* mint green */
  opacity: 0.5;
  top: 70px;
  right: -70px;
}
.b-blue {
  width: 380px;
  height: 380px;
  background: #cfe6fe;
  opacity: 0.45;
  bottom: -130px;
  left: 34%;
}
.b-amber {
  width: 320px;
  height: 320px;
  background: #ffe9a8;
  opacity: 0.4;
  top: 46%;
  right: 24%;
}
/* Keep chrome white, let the gradient show through the content area only. */
.app-layout {
  position: relative;
  z-index: 1;
  background-color: transparent;
}
.app-layout :deep(.n-layout),
.app-layout :deep(.n-layout-content),
.app-layout :deep(.n-layout-content .n-layout-scroll-container) {
  background-color: transparent;
}

.topbar {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 11px;
}
.brand-mark {
  width: 32px;
  height: 32px;
  border-radius: 9px;
  background: linear-gradient(135deg, #ffe066, #f2c200);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 6px rgba(242, 194, 0, 0.35);
}
.brand-text {
  display: flex;
  flex-direction: column;
  line-height: 1.15;
}
.brand-name {
  font-size: 15px;
  font-weight: 650;
  color: #1f2933;
  letter-spacing: -0.01em;
}
.brand-sub {
  font-size: 11px;
  color: #9aa5b1;
}
.user {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 9px;
  transition: background 0.15s ease;
}
.user:hover {
  background: #f4f6f9;
}

/* --- Sidebar --- */
.sider {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 18px 14px;
}
.nav-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  color: #aeb6c0;
  padding: 0 10px 10px;
  text-transform: uppercase;
}
.nav {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.nav-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  height: 46px;
  padding: 0 14px;
  border: none;
  background: transparent;
  border-radius: 11px;
  cursor: pointer;
  color: #52606d;
  font-size: 14.5px;
  font-weight: 500;
  text-align: left;
  transition: background 0.18s ease, color 0.18s ease, transform 0.18s ease;
}
.nav-item:hover {
  background: #f3f5f8;
  color: #1f2933;
  transform: translateX(2px);
}
.nav-item.active {
  background: rgba(242, 194, 0, 0.15);
  color: #1f2933;
  font-weight: 600;
}
.nav-icon {
  flex: none;
  transition: transform 0.18s ease;
}
.nav-item.active .nav-icon {
  color: #c79a00;
}
.nav-item:hover .nav-icon {
  transform: scale(1.1);
}
/* Animated left accent that grows in on the active item. */
.nav-indicator {
  position: absolute;
  left: 4px;
  top: 50%;
  width: 3px;
  height: 0;
  border-radius: 3px;
  background: #f2c200;
  transform: translateY(-50%);
  transition: height 0.22s ease;
}
.nav-item.active .nav-indicator {
  height: 20px;
}

.sider-footer {
  margin-top: auto;
  padding-top: 14px;
}
.status-card {
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 12px 14px;
  background: #f7f9fb;
  border: 1px solid #eef1f4;
  border-radius: 12px;
}
.status-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex: none;
}
.status-dot.pulse {
  animation: pulseRing 1.8s infinite;
}
.status-body {
  display: flex;
  flex-direction: column;
  line-height: 1.25;
}
.status-iface {
  font-size: 13px;
  font-weight: 600;
  color: #1f2933;
}
.status-text {
  font-size: 11.5px;
  color: #9aa5b1;
}

/* --- Content --- */
.page {
  max-width: 1240px;
  margin: 0 auto;
}

/* Route transition */
.page-enter-active,
.page-leave-active {
  transition: opacity 0.24s ease, transform 0.24s ease;
}
.page-enter-from {
  opacity: 0;
  transform: translateY(10px);
}
.page-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
</style>
