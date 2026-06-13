<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NCard, NForm, NFormItem, NInput, NInputNumber, NButton, NSpace, NText, NTag, NAlert,
  NInputGroup, NSwitch, useMessage,
} from 'naive-ui'
import { api } from '../api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)

// --- IPv4 forwarding ---
const forwarding = ref<boolean | null>(null)
const enabling = ref(false)

// --- boot autostart (systemd wg-quick@wg0) ---
const autostart = ref(false)
const togglingAuto = ref(false)

async function onToggleAutostart(val: boolean) {
  togglingAuto.value = true
  try {
    const r = await api.setAutostart(val)
    autostart.value = r.autostart
    message.success(val ? '已开启开机自启' : '已关闭开机自启')
  } catch (e) {
    message.error((e as Error).message)
    autostart.value = !val // revert the switch to reflect the real state
  } finally {
    togglingAuto.value = false
  }
}

async function loadForwarding() {
  try {
    const s = await api.system()
    forwarding.value = s.ipv4Forwarding
  } catch {
    /* ignore */
  }
}

async function enableForwarding() {
  enabling.value = true
  try {
    await api.enableIPForward()
    forwarding.value = true
    message.success('IPv4 转发已开启')
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    enabling.value = false
  }
}

// --- account / password ---
const acctUser = ref('')
const curPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const changing = ref(false)

async function loadAccount() {
  try {
    const me = await api.me()
    acctUser.value = me.username
  } catch {
    /* ignore */
  }
}

async function changePassword() {
  if (!newPassword.value) {
    message.error('请输入新密码')
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    message.error('两次输入的新密码不一致')
    return
  }
  changing.value = true
  try {
    await api.changePassword(curPassword.value, acctUser.value.trim(), newPassword.value)
    message.success('登录信息已更新')
    curPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    changing.value = false
  }
}
const configured = ref(true)
const live = ref(false)

// Form state (addresses / DNS edited as comma-separated text).
const address = ref('')
const listenPort = ref<number | null>(51820)
const mtu = ref<number | null>(null)
const endpointHost = ref('')
const detecting = ref(false)

async function detectIP() {
  detecting.value = true
  try {
    const r = await api.detectPublicIP()
    endpointHost.value = r.ip
    message.success('已获取公网 IP：' + r.ip)
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    detecting.value = false
  }
}

function splitList(s: string): string[] {
  return s.split(',').map((x) => x.trim()).filter(Boolean)
}

// --- inline validation for the interface fields ---
function isValidCIDR(s: string): boolean {
  const m = s.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\/(\d{1,2})$/)
  if (!m) return false
  if ([m[1], m[2], m[3], m[4]].some((o) => Number(o) > 255)) return false
  const prefix = Number(m[5])
  return prefix >= 0 && prefix <= 32
}

const addrError = computed(() => {
  const items = splitList(address.value)
  if (items.length === 0) return '请填写组网网段'
  const bad = items.find((a) => !isValidCIDR(a))
  return bad ? `网段格式不正确：${bad}（应形如 10.7.0.1/24）` : ''
})
const portError = computed(() => {
  const p = listenPort.value
  if (p == null) return '请填写监听端口'
  if (p < 1 || p > 65535) return '监听端口需在 1-65535 之间'
  return ''
})
const mtuError = computed(() => {
  const m = mtu.value
  if (m == null || m === 0) return '' // optional
  return m < 1280 || m > 1500 ? 'MTU 需在 1280-1500 之间（或留空用默认）' : ''
})

async function load() {
  loading.value = true
  try {
    const s = await api.getSettings()
    address.value = (s.address ?? []).join(', ')
    listenPort.value = s.listenPort || null
    mtu.value = s.mtu || null
    endpointHost.value = s.endpointHost ?? ''
    configured.value = s.configured
    live.value = s.live
    autostart.value = s.autostart
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

async function save() {
  // Validate before submitting; pop a specific message and abort if invalid.
  const err = addrError.value || portError.value || mtuError.value
  if (err) {
    message.error(err)
    return
  }
  saving.value = true
  try {
    const res = await api.updateSettings({
      address: splitList(address.value),
      listenPort: listenPort.value ?? 0,
      mtu: mtu.value ?? 0,
      endpointHost: endpointHost.value.trim(),
    })
    if (res.applyError) {
      message.warning('已保存，但应用到接口失败：' + res.applyError)
    } else if (res.restarted) {
      message.success('已保存，接口已重启生效')
    } else if (res.applied) {
      message.success('已保存并热加载生效')
    } else {
      message.success('已保存（接口未运行，下次启动生效）')
    }
    await load()
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  load()
  loadAccount()
  loadForwarding()
})
</script>

<template>
  <n-space vertical :size="22">
    <div class="page-head">
      <div>
        <div class="page-title">设置</div>
        <div class="page-desc">组网网段、监听端口与客户端配置参数</div>
      </div>
      <n-tag v-if="!configured" round size="small" :bordered="false" type="error">未配置</n-tag>
      <n-tag v-else-if="!live" round size="small" :bordered="false" type="warning">接口未运行</n-tag>
      <n-tag v-else round size="small" :bordered="false" type="success">运行中</n-tag>
    </div>

    <n-card title="接口配置" :bordered="true">
      <template #header-extra>
        <n-text depth="3" style="font-size: 12px">写入 wg0.conf</n-text>
      </template>
      <n-form label-placement="left" :label-width="110" style="max-width: 560px">
        <n-form-item
          label="组网网段"
          :validation-status="addrError ? 'error' : undefined"
          :feedback="addrError || undefined"
        >
          <n-input v-model:value="address" placeholder="10.7.0.1/24" />
        </n-form-item>
        <n-form-item
          label="监听端口"
          :validation-status="portError ? 'error' : undefined"
          :feedback="portError || undefined"
        >
          <!-- No min/max clamping: keep the user's raw value so validation can
               catch out-of-range input and pop a message instead of silently
               snapping it to a valid number. -->
          <n-input-number v-model:value="listenPort" :show-button="false" style="width: 200px" />
        </n-form-item>
        <n-form-item
          label="MTU"
          :validation-status="mtuError ? 'error' : undefined"
          :feedback="mtuError || undefined"
        >
          <n-input-number v-model:value="mtu" :show-button="false" placeholder="默认" style="width: 200px" />
        </n-form-item>
      </n-form>
      <n-alert type="info" :bordered="false" style="margin-top: 4px">
        修改「组网网段」或「MTU」会<strong>重启接口</strong>（已连接的客户端会短暂断开）；仅改端口为热加载，不断线。
      </n-alert>
    </n-card>

    <n-card title="IPv4 转发" :bordered="true">
      <template #header-extra>
        <n-tag v-if="forwarding !== null" size="small" round :bordered="false" :type="forwarding ? 'success' : 'error'">
          {{ forwarding ? '已开启' : '未开启' }}
        </n-tag>
      </template>
      <n-space align="center" justify="space-between" :wrap="false">
        <n-text depth="3" style="font-size: 13px; max-width: 460px">
          异地组网必须开启 IPv4 转发，否则客户端之间无法互相访问。开启后立即生效，并写入 sysctl 持久化（重启仍有效）。
        </n-text>
        <n-button type="primary" :loading="enabling" :disabled="forwarding === true" @click="enableForwarding">
          {{ forwarding ? '已开启' : '开启转发' }}
        </n-button>
      </n-space>
    </n-card>

    <n-card title="开机自启" :bordered="true">
      <template #header-extra>
        <n-tag size="small" round :bordered="false" :type="autostart ? 'success' : 'default'">
          {{ autostart ? '已开启' : '已关闭' }}
        </n-tag>
      </template>
      <n-space align="center" justify="space-between" :wrap="false">
        <n-text depth="3" style="font-size: 13px; max-width: 460px">
          开启后，服务器重启时会自动拉起 WireGuard 接口（systemd：wg-quick@wg0），无需手动到总览点「启动」。
        </n-text>
        <n-switch :value="autostart" :loading="togglingAuto" @update:value="onToggleAutostart" />
      </n-space>
    </n-card>

    <n-card title="客户端配置参数" :bordered="true">
      <template #header-extra>
        <n-text depth="3" style="font-size: 12px">用于生成客户端配置</n-text>
      </template>
      <n-form label-placement="left" :label-width="110" style="max-width: 560px">
        <n-form-item label="公网地址">
          <n-input-group>
            <n-input v-model:value="endpointHost" placeholder="vpn.example.com 或公网 IP" />
            <n-button :loading="detecting" @click="detectIP">自动获取</n-button>
          </n-input-group>
        </n-form-item>
      </n-form>
      <n-text depth="3" style="font-size: 12px">
        公网地址是客户端连接的 Endpoint（写入每个客户端配置）。「自动获取」会让服务器探测自身公网 IP。客户端的 AllowedIPs 会自动设为组网网段，仅路由内网流量。
      </n-text>
    </n-card>

    <n-space justify="end" style="max-width: 100%">
      <n-button @click="load">重置</n-button>
      <n-button type="primary" :loading="saving" :disabled="loading" @click="save">保存设置</n-button>
    </n-space>

    <n-card title="面板账号" :bordered="true">
      <template #header-extra>
        <n-text depth="3" style="font-size: 12px">登录用户名与密码</n-text>
      </template>
      <n-form label-placement="left" :label-width="110" style="max-width: 560px">
        <n-form-item label="用户名">
          <n-input v-model:value="acctUser" placeholder="admin" />
        </n-form-item>
        <n-form-item label="当前密码">
          <n-input v-model:value="curPassword" type="password" show-password-on="click" placeholder="验证身份" />
        </n-form-item>
        <n-form-item label="新密码">
          <n-input v-model:value="newPassword" type="password" show-password-on="click" placeholder="设置新密码" />
        </n-form-item>
        <n-form-item label="确认新密码">
          <n-input v-model:value="confirmPassword" type="password" show-password-on="click" placeholder="再次输入新密码" @keyup.enter="changePassword" />
        </n-form-item>
      </n-form>
      <n-space justify="end">
        <n-button type="primary" :loading="changing" @click="changePassword">更新登录信息</n-button>
      </n-space>
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
</style>
