<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import {
  NCard, NDataTable, NButton, NSpace, NText, NTag, NTooltip, NModal, NInput,
  NDropdown, NAlert, NSelect, NFormItem, useMessage, useDialog,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { api, fmtBytes, fmtSince, type AddResult, type Client, type ClientConfigView, type Pool } from '../api'
import { lookupIP, endpointTip } from '../ipgeo'

const message = useMessage()
const dialog = useDialog()
const clients = ref<Client[]>([])
const live = ref(false)
const configured = ref(true)
const loading = ref(false)

const banner = computed(() => {
  if (!configured.value) return { type: 'error' as const, text: '未配置 · 找不到 wg0.conf' }
  if (!live.value) return { type: 'warning' as const, text: '接口未运行 · 握手与流量暂不可用，但仍可增删客户端' }
  return null
})

// Split a free-text list of CIDRs (comma- or newline-separated) into entries.
function parseSubnets(s: string): string[] {
  return s.split(/[\n,]/).map((x) => x.trim()).filter(Boolean)
}

// --- create ---
const createOpen = ref(false)
const createName = ref('')
const createAddress = ref('')
const createSubnets = ref('')
const creating = ref(false)
const pools = ref<Pool[]>([])
const selectedPool = ref<string>('')

const poolOptions = computed(() =>
  pools.value.map((p) => ({
    label: p.nextFree ? `${p.network} · 下一空闲 ${p.nextFree}` : `${p.network} · 已满`,
    value: p.cidr,
  })),
)
const nextFreeHint = computed(() => {
  const p = pools.value.find((x) => x.cidr === selectedPool.value)
  return p?.nextFree ? `留空自动分配（下一个空闲：${p.nextFree}）` : '留空自动分配'
})

// --- generated-config modal (shown once after create) ---
const created = ref<AddResult | null>(null)

async function openCreate() {
  createName.value = ''
  createAddress.value = ''
  createSubnets.value = ''
  try {
    const n = await api.network()
    pools.value = n.pools
    selectedPool.value = pools.value[0]?.cidr ?? ''
  } catch {
    pools.value = []
  }
  createOpen.value = true
}

async function submitCreate() {
  creating.value = true
  try {
    const res = await api.createClient(
      createName.value.trim(),
      createAddress.value.trim(),
      parseSubnets(createSubnets.value),
    )
    createOpen.value = false
    created.value = res
    if (res.reloadError) {
      message.warning('已写入配置，但热加载失败：' + res.reloadError)
    } else {
      message.success('客户端已创建并生效')
    }
    await load()
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    creating.value = false
  }
}

// Copy helper that works over plain HTTP too. navigator.clipboard is only
// exposed in secure contexts (HTTPS or localhost); on a public-IP HTTP panel
// it's undefined, so we fall back to a temporary <textarea> + execCommand.
function legacyCopy(text: string): boolean {
  const ta = document.createElement('textarea')
  ta.value = text
  // Keep it off-screen but still selectable.
  ta.style.position = 'fixed'
  ta.style.top = '-9999px'
  ta.style.left = '-9999px'
  ta.setAttribute('readonly', '')
  // Append INSIDE the current focus context (the copy button lives in the
  // modal). Naive UI's modal has a focus trap: appending to document.body and
  // focusing the textarea makes the trap yank focus back into the modal,
  // clearing our selection before execCommand runs — execCommand then returns
  // true but copies nothing. Mounting within the trap avoids that fight.
  const host = (document.activeElement as HTMLElement | null)?.parentElement ?? document.body
  host.appendChild(ta)
  ta.focus()
  ta.select()
  ta.setSelectionRange(0, text.length)
  let ok = false
  try {
    ok = document.execCommand('copy')
  } catch {
    ok = false
  }
  host.removeChild(ta)
  return ok
}

async function copyText(text?: string) {
  if (!text) return
  // Prefer the async Clipboard API when it's available (secure context).
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      message.success('已复制到剪贴板')
      return
    } catch {
      /* fall through to legacy path */
    }
  }
  if (legacyCopy(text)) {
    message.success('已复制到剪贴板')
  } else {
    message.error('复制失败，请手动选择文本')
  }
}

// --- view existing config ---
const viewing = ref<ClientConfigView | null>(null)

async function openView(row: Client) {
  try {
    viewing.value = await api.clientConfig(row.publicKey)
  } catch (e) {
    message.error((e as Error).message)
  }
}

const endpointPlaceholder = computed(() =>
  created.value?.configText.includes('<SERVER_PUBLIC_IP>') ?? false,
)

// --- rename ---
const renameOpen = ref(false)
const renameTarget = ref<Client | null>(null)
const renameName = ref('')

function openRename(row: Client) {
  renameTarget.value = row
  renameName.value = row.name
  renameOpen.value = true
}

async function submitRename() {
  if (!renameTarget.value) return
  try {
    await api.renameClient(renameTarget.value.publicKey, renameName.value.trim())
    renameOpen.value = false
    message.success('已重命名')
    await load()
  } catch (e) {
    message.error((e as Error).message)
  }
}

// --- announced subnets (LANs behind a client) ---
const subnetsOpen = ref(false)
const subnetsTarget = ref<Client | null>(null)
const subnetsText = ref('')
const savingSubnets = ref(false)

function openSubnets(row: Client) {
  subnetsTarget.value = row
  subnetsText.value = (row.subnets ?? []).join('\n')
  subnetsOpen.value = true
}

async function submitSubnets() {
  if (!subnetsTarget.value) return
  savingSubnets.value = true
  try {
    const res = await api.setClientSubnets(subnetsTarget.value.publicKey, parseSubnets(subnetsText.value))
    subnetsOpen.value = false
    if (res.reloadError) {
      message.warning('已保存，但热加载失败：' + res.reloadError)
    } else {
      message.success('宣告内网已更新')
    }
    await load()
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    savingSubnets.value = false
  }
}

// --- delete ---
async function remove(row: Client) {
  try {
    await api.deleteClient(row.publicKey)
    message.success('已删除')
    await load()
  } catch (e) {
    message.error((e as Error).message)
  }
}

function confirmDelete(row: Client) {
  dialog.warning({
    title: '删除客户端',
    content: `确认删除「${row.name || '未命名'}」？此操作不可撤销。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: () => remove(row),
  })
}

// --- "更多" actions menu ---
const moreOptions = [
  { label: '宣告内网', key: 'subnets' },
  { label: '重命名', key: 'rename' },
  { type: 'divider', key: 'd1' },
  { label: () => h('span', { style: 'color:#e5484d' }, '删除'), key: 'delete' },
]

function onMoreSelect(key: string, row: Client) {
  if (key === 'subnets') openSubnets(row)
  else if (key === 'rename') openRename(row)
  else if (key === 'delete') confirmDelete(row)
}

const columns: DataTableColumns<Client> = [
  { title: '名称', key: 'name', render: (r) => r.name || h(NText, { depth: 3 }, () => '(未命名)') },
  { title: 'IP', key: 'allowedIPs', render: (r) => (r.allowedIPs ?? []).join(', ') || '—' },
  {
    title: '宣告内网',
    key: 'subnets',
    render: (r) => {
      const subs = r.subnets ?? []
      if (subs.length === 0) return h(NText, { depth: 3 }, () => '—')
      if (subs.length === 1) {
        return h(NTag, { size: 'small', bordered: false, type: 'info' }, () => subs[0])
      }
      // More than one: show the first + a "+N" badge; reveal all on hover.
      return h(NTooltip, null, {
        trigger: () =>
          h(NSpace, { size: 4, wrapItem: false, align: 'center', wrap: false }, () => [
            h(NTag, { size: 'small', bordered: false, type: 'info' }, () => subs[0]),
            h(NTag, { size: 'small', bordered: false, round: true, style: 'cursor: default' },
              () => `+${subs.length - 1}`),
          ]),
        default: () =>
          h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
            subs.map((s) => h('span', null, s))),
      })
    },
  },
  {
    title: 'Endpoint',
    key: 'endpoint',
    render: (r) =>
      r.endpoint
        ? h(
            NTooltip,
            { onUpdateShow: (show: boolean) => { if (show) lookupIP(r.endpoint) } },
            { trigger: () => r.endpoint, default: () => endpointTip(r.endpoint) },
          )
        : h(NText, { depth: 3 }, () => '—'),
  },
  {
    title: '最近握手',
    key: 'lastHandshake',
    render: (r) =>
      h(NTooltip, null, {
        trigger: () => fmtSince(r.lastHandshake),
        default: () => (r.lastHandshake ? new Date(r.lastHandshake).toLocaleString() : '从未握手'),
      }),
  },
  { title: '上行', key: 'txBytes', render: (r) => fmtBytes(r.txBytes) },
  { title: '下行', key: 'rxBytes', render: (r) => fmtBytes(r.rxBytes) },
  {
    title: '状态',
    key: 'online',
    render: (r) =>
      h(NTag, { type: r.online ? 'success' : 'default', size: 'small', bordered: false },
        () => (r.online ? '在线' : '离线')),
  },
  {
    title: '操作',
    key: 'actions',
    render: (r) =>
      h(NSpace, { size: 8, wrapItem: false, align: 'center' }, () => [
        h(NButton, { size: 'small', tertiary: true, type: 'primary', onClick: () => openView(r) }, () => '查看'),
        h(
          NDropdown,
          {
            trigger: 'click',
            options: moreOptions,
            onSelect: (key: string) => onMoreSelect(key, r),
          },
          () => h(NButton, { size: 'small', tertiary: true }, () => '更多'),
        ),
      ]),
  },
]

async function load() {
  loading.value = true
  try {
    const res = await api.clients()
    clients.value = res.clients
    live.value = res.live
    configured.value = res.configured
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <n-space vertical :size="22">
    <div class="page-head">
      <div>
        <div class="page-title">客户端</div>
        <div class="page-desc">管理 WireGuard 对端，自动分配地址并生成配置</div>
      </div>
      <n-space align="center" :size="10">
        <n-tag v-if="banner" round size="small" :bordered="false" :type="banner.type">{{ banner.text }}</n-tag>
        <n-button size="small" secondary :loading="loading" @click="load">刷新</n-button>
        <n-button type="primary" :disabled="!configured" @click="openCreate">新增客户端</n-button>
      </n-space>
    </div>

    <n-card :bordered="true">
      <n-data-table
        :columns="columns"
        :data="clients"
        :loading="loading"
        :bordered="false"
        :row-key="(row: Client) => row.publicKey"
      />
    </n-card>

    <!-- Create -->
    <n-modal v-model:show="createOpen" preset="card" title="新增客户端" style="width: 460px">
      <n-space vertical :size="4">
        <n-form-item label="名称" :show-feedback="false">
          <n-input v-model:value="createName" placeholder="如：我的手机" @keyup.enter="submitCreate" />
        </n-form-item>
        <n-form-item v-if="pools.length" label="网段" :show-feedback="false">
          <n-select v-model:value="selectedPool" :options="poolOptions" />
        </n-form-item>
        <n-form-item label="IP 地址" :show-feedback="false">
          <n-input v-model:value="createAddress" :placeholder="nextFreeHint" @keyup.enter="submitCreate" />
        </n-form-item>
        <n-form-item label="宣告内网" :show-feedback="false">
          <n-input v-model:value="createSubnets" placeholder="可选，如 192.168.1.0/24（多个用逗号分隔）" @keyup.enter="submitCreate" />
        </n-form-item>
        <n-text depth="3" style="font-size: 12px; padding: 4px 0 12px">
          IP 留空则自动分配该网段下一个空闲地址；也可手动指定（须在网段内且未被占用）。「宣告内网」填该客户端背后的局域网网段，其它客户端即可访问到它。
        </n-text>
        <n-space justify="end">
          <n-button @click="createOpen = false">取消</n-button>
          <n-button type="primary" :loading="creating" @click="submitCreate">创建</n-button>
        </n-space>
      </n-space>
    </n-modal>

    <!-- Generated config (one-time) -->
    <n-modal
      :show="created !== null"
      preset="card"
      title="客户端配置（私钥仅显示这一次）"
      style="width: 560px"
      @update:show="(v: boolean) => { if (!v) created = null }"
    >
      <n-space vertical :size="12">
        <n-alert type="warning" :bordered="false">
          私钥不会被服务端保存，请立即复制保存。关闭后无法再次查看。
        </n-alert>
        <n-alert v-if="endpointPlaceholder" type="info" :bordered="false">
          配置里的 Endpoint 还是占位符。请到「设置」页填写公网地址后重新生成。
        </n-alert>
        <div v-if="created?.qrCode" class="qr-wrap">
          <img :src="created.qrCode" class="qr" alt="QR" />
          <span class="qr-tip">手机 App 扫码即可导入</span>
        </div>
        <n-input
          type="textarea"
          :value="created?.configText"
          readonly
          :autosize="{ minRows: 8, maxRows: 16 }"
        />
        <n-space justify="end">
          <n-button @click="created = null">关闭</n-button>
          <n-button type="primary" @click="copyText(created?.configText)">复制配置</n-button>
        </n-space>
      </n-space>
    </n-modal>

    <!-- View existing config -->
    <n-modal
      :show="viewing !== null"
      preset="card"
      :title="`配置 · ${viewing?.name || '未命名'}`"
      style="width: 560px"
      @update:show="(v: boolean) => { if (!v) viewing = null }"
    >
      <n-space vertical :size="12">
        <n-alert v-if="viewing && !viewing.hasPrivateKey" type="warning" :bordered="false">
          此客户端的私钥未保存（创建于"保存私钥"功能之前，或为手动添加），下方私钥为占位符，需用你首次保存的副本替换后才能导入。
        </n-alert>
        <div v-if="viewing?.qrCode" class="qr-wrap">
          <img :src="viewing.qrCode" class="qr" alt="QR" />
          <span class="qr-tip">手机 App 扫码即可导入</span>
        </div>
        <n-input
          type="textarea"
          :value="viewing?.configText"
          readonly
          :autosize="{ minRows: 8, maxRows: 16 }"
        />
        <n-space justify="end">
          <n-button @click="viewing = null">关闭</n-button>
          <n-button type="primary" @click="copyText(viewing?.configText)">复制配置</n-button>
        </n-space>
      </n-space>
    </n-modal>

    <!-- Announced subnets -->
    <n-modal
      v-model:show="subnetsOpen"
      preset="card"
      :title="`宣告内网 · ${subnetsTarget?.name || '未命名'}`"
      style="width: 500px"
    >
      <n-space vertical :size="12">
        <n-text depth="3" style="font-size: 13px">
          填写该客户端背后的局域网网段（每行或逗号分隔一个，如 192.168.1.0/24）。保存后，其它客户端的配置会自动加上这些路由，从而能访问该客户端背后的内网。
        </n-text>
        <n-input
          v-model:value="subnetsText"
          type="textarea"
          placeholder="192.168.1.0/24"
          :autosize="{ minRows: 3, maxRows: 8 }"
        />
        <n-alert type="info" :bordered="false" :show-icon="false">
          改动会影响其它客户端的路由：它们需重新「查看」并导入最新配置才能访问新宣告的内网。另外，作为网关的该客户端机器需自行开启 IP 转发并对内网做 NAT（参见说明）。
        </n-alert>
        <n-space justify="end">
          <n-button @click="subnetsOpen = false">取消</n-button>
          <n-button type="primary" :loading="savingSubnets" @click="submitSubnets">保存</n-button>
        </n-space>
      </n-space>
    </n-modal>

    <!-- Rename -->
    <n-modal v-model:show="renameOpen" preset="card" title="重命名客户端" style="width: 420px">
      <n-space vertical :size="12">
        <n-input v-model:value="renameName" placeholder="新名称" @keyup.enter="submitRename" />
        <n-space justify="end">
          <n-button @click="renameOpen = false">取消</n-button>
          <n-button type="primary" @click="submitRename">保存</n-button>
        </n-space>
      </n-space>
    </n-modal>
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
.qr-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
}
.qr {
  width: 200px;
  height: 200px;
  border: 1px solid #eef1f4;
  border-radius: 12px;
  padding: 8px;
  background: #fff;
}
.qr-tip {
  font-size: 12px;
  color: #9aa5b1;
}
</style>
