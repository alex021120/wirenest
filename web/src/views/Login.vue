<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui'
import { api } from '../api'

const router = useRouter()
const route = useRoute()
const message = useMessage()

const username = ref('admin')
const password = ref('')
const loading = ref(false)

async function submit() {
  loading.value = true
  try {
    await api.login(username.value, password.value)
    const redirect = (route.query.redirect as string) || '/overview'
    router.push(redirect)
  } catch (e) {
    message.error((e as Error).message || '登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <div class="blob blob-a" />
    <div class="blob blob-b" />
    <div class="blob blob-c" />

    <div class="login-card">
      <div class="login-head">
        <div class="brand-mark">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="#1F2933" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 2L3 7v5c0 5 3.8 8.4 9 10 5.2-1.6 9-5 9-10V7l-9-5z" />
          </svg>
        </div>
        <h1 class="title">WireNest</h1>
        <p class="subtitle">WireGuard 异地组网管理面板</p>
      </div>

      <n-form @submit.prevent="submit">
        <n-form-item label="用户名">
          <n-input v-model:value="username" placeholder="用户名" size="large" />
        </n-form-item>
        <n-form-item label="密码">
          <n-input
            v-model:value="password"
            type="password"
            show-password-on="click"
            placeholder="密码"
            size="large"
            @keyup.enter="submit"
          />
        </n-form-item>
        <n-button
          type="primary"
          size="large"
          block
          :loading="loading"
          attr-type="submit"
          @click="submit"
        >
          登录
        </n-button>
      </n-form>
    </div>
  </div>
</template>

<style scoped>
.login-wrap {
  position: relative;
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(160deg, #fbfcfe 0%, #f3f6fb 100%);
  overflow: hidden;
}
/* Soft, blurred brand-tinted blobs for a fresh, airy backdrop. */
.blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(70px);
  opacity: 0.55;
}
.blob-a {
  width: 360px;
  height: 360px;
  background: #ffe88a;
  top: -90px;
  right: -60px;
}
.blob-b {
  width: 320px;
  height: 320px;
  background: #cfeafe;
  bottom: -100px;
  left: -70px;
}
.blob-c {
  width: 300px;
  height: 300px;
  background: #b6f0d8; /* mint green */
  bottom: -60px;
  right: 8%;
  opacity: 0.5;
}
.login-card {
  position: relative;
  z-index: 1;
  width: 380px;
  background: #fff;
  border: 1px solid #eef1f4;
  border-radius: 18px;
  padding: 34px 32px;
  box-shadow: 0 18px 50px rgba(17, 24, 39, 0.08);
}
.login-head {
  text-align: center;
  margin-bottom: 22px;
}
.brand-mark {
  width: 52px;
  height: 52px;
  border-radius: 15px;
  margin: 0 auto 14px;
  background: linear-gradient(135deg, #ffe066, #f2c200);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 6px 16px rgba(242, 194, 0, 0.4);
}
.title {
  font-size: 19px;
  font-weight: 650;
  color: #1f2933;
  margin: 0 0 4px;
}
.subtitle {
  font-size: 13px;
  color: #9aa5b1;
  margin: 0;
}
</style>
