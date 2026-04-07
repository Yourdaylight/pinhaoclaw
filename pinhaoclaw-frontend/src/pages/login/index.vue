<template>
  <!-- #ifdef H5 -->
  <div class="h5-login-page">
    <div class="login-wrapper">
      <el-card class="login-card" shadow="hover">
        <template #header>
          <div class="logo-area">
            <span class="logo-emoji">🦞</span>
            <h1 class="brand">拼好虾 PinHaoClaw</h1>
            <p class="slogan">隔离式 AI 龙虾 SaaS 平台</p>
          </div>
        </template>

        <div v-if="authMode === 'casdoor'" class="casdoor-mode">
          <el-alert
            title="已启用 Casdoor 统一认证"
            type="success"
            show-icon
            :closable="false"
          />

          <div class="casdoor-copy">
            <p>登录与注册都由统一授权中心处理。</p>
            <p>
              新用户会直接注册到
              <strong>{{ authConfig.organization || 'JQ' }}</strong>
              组织下。
            </p>
            <p class="hint">{{ authConfig.register_hint || '在统一认证页点击注册即可完成开户。' }}</p>
          </div>

          <el-button
            type="primary"
            size="large"
            class="login-btn"
            :loading="loading"
            @click="goCasdoorLogin"
          >
            前往统一认证中心
          </el-button>

          <el-alert
            v-if="errorMsg"
            :title="errorMsg"
            type="error"
            show-icon
            :closable="false"
            style="margin-top: 12px"
          />
        </div>

        <el-form v-else @submit.prevent="doLogin" label-position="top">
          <el-form-item label="邀请码">
            <el-input
              v-model="inviteCode"
              placeholder="请输入邀请码"
              maxlength="20"
              clearable
              size="large"
              :prefix-icon="KeyIcon"
              @keyup.enter="doLogin"
            />
          </el-form-item>

          <el-form-item>
            <el-button
              type="primary"
              size="large"
              :loading="loading"
              class="login-btn"
              @click="doLogin"
            >
              进入控制台
            </el-button>
          </el-form-item>

          <el-alert
            v-if="errorMsg"
            :title="errorMsg"
            type="error"
            show-icon
            :closable="false"
            style="margin-top: 8px"
          />
        </el-form>
      </el-card>
    </div>
  </div>
  <!-- #endif -->

  <!-- #ifndef H5 -->
  <view class="mp-login-container">
    <view class="bg-gradient" />
    <view class="content">
      <view class="logo-section">
        <text class="logo-emoji">🦞</text>
        <text class="logo-title">拼好虾</text>
        <text class="logo-subtitle">你的专属 AI 龙虾</text>
      </view>

      <view class="card">
        <template v-if="authMode === 'casdoor'">
          <text class="card-title">已启用统一认证</text>
          <text class="casdoor-tip">当前 Casdoor 登录流程优先支持 H5 浏览器访问，请使用部署后的 Web 地址登录或注册。</text>
          <text class="casdoor-tip minor">注册完成后用户会自动进入 {{ authConfig.organization || 'JQ' }} 组织。</text>
        </template>
        <template v-else>
          <text class="card-title">输入邀请码</text>
          <input
            class="invite-input"
            v-model="inviteCode"
            placeholder="请输入邀请码"
            placeholder-class="input-placeholder"
            maxlength="20"
            @confirm="doLogin"
          />
          <button
            class="login-btn"
            :class="{ disabled: loading }"
            :disabled="loading"
            @click="doLogin"
          >
            {{ loading ? "验证中..." : "进入龙虾窝" }}
          </button>
          <text v-if="errorMsg" class="error-msg">{{ errorMsg }}</text>
        </template>
      </view>
    </view>
  </view>
  <!-- #endif -->
</template>

<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useUserStore } from "../../stores/user";
import { authApi, type AuthConfigResponse } from "../../api/auth";
// #ifdef H5
import { Key } from "@element-plus/icons-vue";
const KeyIcon = Key;
// #endif

const userStore = useUserStore();
const inviteCode = ref("");
const loading = ref(false);
const errorMsg = ref("");
const authMode = ref<"invite" | "casdoor">("invite");
const authConfig = ref<AuthConfigResponse>({
  mode: "invite",
  casdoor_enabled: false,
});

onMounted(async () => {
  if (userStore.isLoggedIn) {
    uni.reLaunch({ url: "/pages/panel/index" });
    return;
  }

  try {
    const cfg = await authApi.config();
    authConfig.value = cfg;
    authMode.value = cfg.mode || "invite";
  } catch {
    authMode.value = "invite";
  }

  // #ifdef H5
  if (authMode.value !== "casdoor") {
    const search = window.location.search;
    const params = new URLSearchParams(
      search.replace("#/", "").replace(/^.*\?/, "?")
    );
    const code = params.get("code");
    if (code) inviteCode.value = code;
  }
  // #endif
});

function goCasdoorLogin() {
  loading.value = true;
  errorMsg.value = "";
  // #ifdef H5
  window.location.href = authConfig.value.login_url || "/api/auth/login/casdoor";
  // #endif
}

async function doLogin() {
  if (authMode.value === "casdoor") {
    goCasdoorLogin();
    return;
  }

  const code = inviteCode.value.trim();
  if (!code) {
    errorMsg.value = "请输入邀请码";
    return;
  }
  loading.value = true;
  errorMsg.value = "";
  uni.showLoading({ title: "验证中..." });

  await userStore
    .login(code)
    .then(() => {
      uni.hideLoading();
      uni.reLaunch({ url: "/pages/panel/index" });
    })
    .catch((err: Error) => {
      uni.hideLoading();
      errorMsg.value = err.message || "邀请码无效";
      loading.value = false;
    });
}
</script>

<style scoped>
/* ── H5 桌面端样式 ── */
/* #ifdef H5 */
.h5-login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  padding: 40px;
}

.login-wrapper {
  width: 420px;
}

.logo-area {
  text-align: center;
}

.logo-emoji {
  font-size: 48px;
  display: block;
  margin-bottom: 8px;
  filter: drop-shadow(0 4px 12px rgba(0, 0, 0, 0.15));
}

.brand {
  font-size: 24px;
  margin: 4px 0;
  background: linear-gradient(135deg, #667eea, #764ba2);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.slogan {
  color: #909399;
  font-size: 13px;
  margin: 0;
}

.casdoor-mode {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.casdoor-copy {
  color: #606266;
  font-size: 14px;
  line-height: 1.8;
}

.casdoor-copy p {
  margin: 0;
}

.casdoor-copy .hint {
  color: #909399;
}

.login-btn {
  width: 100%;
}
/* #endif */

/* ── 小程序端样式 ── */
/* #ifndef H5 */
.mp-login-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  overflow: hidden;
  background: linear-gradient(135deg, #0f0c29, #302b63, #24243e);
}

.bg-gradient {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  background: linear-gradient(135deg, #0f0c29 0%, #302b63 50%, #24243e 100%);
  z-index: -1;
}

.content {
  width: 90%;
  max-width: 600rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 40rpx;
  padding: 60rpx 0;
}

.logo-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16rpx;
}

.logo-emoji {
  font-size: 120rpx;
  filter: drop-shadow(0 8rpx 40rpx rgba(255, 100, 100, 0.5));
  animation: float 3s ease-in-out infinite;
}

@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-16rpx); }
}

.logo-title {
  font-size: 52rpx;
  font-weight: 700;
  color: #fff;
  letter-spacing: 4rpx;
}

.logo-subtitle {
  font-size: 26rpx;
  color: rgba(255, 255, 255, 0.55);
}

.card {
  width: 100%;
  background: rgba(255, 255, 255, 0.07);
  backdrop-filter: blur(20px);
  border: 1rpx solid rgba(255, 255, 255, 0.12);
  border-radius: 40rpx;
  padding: 60rpx 48rpx;
  display: flex;
  flex-direction: column;
  gap: 24rpx;
}

.card-title {
  font-size: 32rpx;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.8);
  text-align: center;
  margin-bottom: 8rpx;
}

.casdoor-tip {
  color: rgba(255, 255, 255, 0.72);
  font-size: 26rpx;
  line-height: 1.8;
  text-align: center;
}

.casdoor-tip.minor {
  color: rgba(255, 255, 255, 0.5);
  font-size: 24rpx;
}

.invite-input {
  width: 100%;
  padding: 30rpx 36rpx;
  border: 2rpx solid rgba(255, 255, 255, 0.15);
  border-radius: 20rpx;
  font-size: 32rpx;
  background: rgba(255, 255, 255, 0.06);
  color: #fff;
  text-align: center;
  letter-spacing: 4rpx;
}

.input-placeholder {
  color: rgba(255, 255, 255, 0.3);
}

.login-btn {
  width: 100%;
  padding: 30rpx;
  border: none;
  border-radius: 20rpx;
  font-size: 30rpx;
  font-weight: 700;
  background: linear-gradient(135deg, #ff6b6b, #ff8e53);
  color: #fff;
  letter-spacing: 2rpx;
}

.login-btn:active {
  transform: scale(0.97);
  opacity: 0.9;
}

.login-btn.disabled {
  opacity: 0.5;
}

.error-msg {
  color: #ff6b6b;
  font-size: 24rpx;
  text-align: center;
  min-height: 36rpx;
}
/* #endif */
</style>
