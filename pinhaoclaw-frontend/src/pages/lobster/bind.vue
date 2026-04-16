<template>
  <view class="bind-page">
    <view class="bind-container">
      <text class="page-title">📱 绑定微信</text>
      <text class="page-desc">请使用微信扫描下方二维码，将微信绑定到这只龙虾上</text>

      <!-- QR 码展示区 -->
      <view class="qr-section" v-if="qrImageSrc">
        <view class="qr-wrapper">
          <image class="qr-image" :src="qrImageSrc" mode="aspectFit" />
        </view>
        <view class="qr-hint-row">
          <text class="qr-hint">打开微信 → 扫一扫</text>
        </view>
      </view>

      <!-- 等待二维码 -->
      <view class="qr-placeholder" v-else-if="!isDone && !isError">
        <view class="qr-loading">
          <view class="loading-ring" />
          <text class="loading-text">正在生成二维码...</text>
        </view>
      </view>

      <!-- 成功 -->
      <view class="result-section success" v-if="isDone">
        <text class="result-icon">✅</text>
        <text class="result-text">微信绑定成功！龙虾已上线 🦞</text>
      </view>

      <!-- 失败 -->
      <view class="result-section error" v-if="isError">
        <text class="result-icon">❌</text>
        <text class="result-text">{{ errorMsg }}</text>
        <button class="btn-retry" @click="startBind">重试</button>
      </view>

      <!-- 进度日志 -->
      <scroll-view class="log-scroll" scroll-y :scroll-top="logScrollTop">
        <view class="log-content">
          <text v-for="(line, i) in logLines" :key="i" class="log-line">{{ line }}</text>
        </view>
      </scroll-view>

      <!-- 关闭按钮 -->
      <button class="btn-close" @click="onClose">
        {{ isDone ? "返回我的龙虾" : "关闭" }}
      </button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { useUserStore } from "../../stores/user";
import { getBaseUrl } from "../../api/request";

const props = defineProps<{ id?: string }>();

const userStore = useUserStore();
const lobsterId = ref("");
const qrImageSrc = ref("");
const logLines = ref<string[]>([]);
const logScrollTop = ref(0);
const isDone = ref(false);
const isError = ref(false);
const errorMsg = ref("");

// SSE 对象（H5）
let eventSource: EventSource | null = null;
// WebSocket 对象（小程序）
let socketTask: UniApp.SocketTask | null = null;

onMounted(() => {
  const pages = getCurrentPages();
  const current = pages[pages.length - 1];
  lobsterId.value = (current as any).options?.id || props.id || "";

  if (!lobsterId.value) {
    errorMsg.value = "龙虾 ID 无效";
    isError.value = true;
    return;
  }
  startBind();
});

onUnmounted(() => {
  cleanup();
});

function cleanup() {
  if (eventSource) { eventSource.close(); eventSource = null; }
  if (socketTask) { socketTask.close({}); socketTask = null; }
}

function addLog(msg: string) {
  if (!msg) return;
  logLines.value.push(msg);
  // 滚动到底部
  setTimeout(() => { logScrollTop.value += 9999; }, 50);
}

function safeJSONParse<T>(data: string, fallback: T): T {
  try { return JSON.parse(data); }
  catch { return fallback; }
}

function handleMessage(event: string, stage: string, message: string, url?: string) {
  if (event === "qrcode" && url) {
    // 通过后端 /api/qrcode 将 URL 转为图片
    const base = getBaseUrl();
    qrImageSrc.value = `${base}/api/qrcode?url=${encodeURIComponent(url)}`;
    addLog("📱 二维码已生成，请用微信扫一扫");
    return;
  }
  if (event === "done") {
    isDone.value = true;
    addLog("✅ " + message);
    setTimeout(onClose, 2500);
    return;
  }
  if (event === "error") {
    isError.value = true;
    errorMsg.value = message;
    addLog("❌ " + message);
    return;
  }
  if (message) addLog(message);
}

function startBind() {
  cleanup();
  isDone.value = false;
  isError.value = false;
  errorMsg.value = "";
  qrImageSrc.value = "";
  logLines.value = ["连接中..."];

  const token = userStore.token;
  const id = lobsterId.value;

  // #ifdef H5
  const sseUrl = `/api/lobsters/${id}/bind?token=${token}`;
  const es = new EventSource(sseUrl);
  eventSource = es;

  es.addEventListener("progress", (e) => {
    const d = safeJSONParse(e.data, { stage: "", message: "" });
    handleMessage("progress", d.stage, d.message);
  });
  es.addEventListener("qrcode", (e) => {
    const d = safeJSONParse(e.data, { stage: "", message: "", url: "" });
    handleMessage("qrcode", d.stage, d.message, d.url);
  });
  es.addEventListener("done", (e) => {
    const d = safeJSONParse(e.data, { stage: "", message: "" });
    handleMessage("done", d.stage, d.message);
    es.close();
  });
  es.addEventListener("error", (e) => {
    if ((e as MessageEvent).data) {
      const d = safeJSONParse((e as MessageEvent).data, { stage: "", message: "" });
      handleMessage("error", d.stage, d.message);
    } else {
      handleMessage("error", "error", "连接断开");
    }
    es.close();
  });
  // #endif

  // #ifdef MP-WEIXIN
  const base = getBaseUrl().replace(/^http/, "ws");
  const wsUrl = `${base}/ws/bind/${id}?token=${token}`;
  const task = uni.connectSocket({
    url: wsUrl,
    success: () => {},
    fail: () => {
      isError.value = true;
      errorMsg.value = "WebSocket 连接失败";
    },
  });
  socketTask = task;

  task.onMessage((res) => {
    const d = safeJSONParse(res.data as string, { event: "", stage: "", message: "" });
    handleMessage(d.event, d.stage, d.message, d.url);
  });

  task.onError(() => {
    isError.value = true;
    errorMsg.value = "连接出错，请重试";
  });

  task.onClose(() => {
    if (!isDone.value) {
      isError.value = true;
      errorMsg.value = "连接意外断开";
    }
  });
  // #endif
}

function onClose() {
  cleanup();
  uni.navigateBack();
}
</script>

<style scoped>
.bind-page {
  min-height: 100vh;
  background: linear-gradient(135deg, #0f0c29, #302b63);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 60rpx;
}

.bind-container {
  width: 90%;
  max-width: 680rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 32rpx;
}

.page-title {
  font-size: 40rpx;
  font-weight: 700;
  color: #fff;
}

.page-desc {
  font-size: 26rpx;
  color: rgba(255, 255, 255, 0.5);
  text-align: center;
}

.qr-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 20rpx;
}

.qr-wrapper {
  padding: 24rpx;
  background: #fff;
  border-radius: 24rpx;
  border: 4rpx solid rgba(255, 107, 107, 0.5);
  box-shadow: 0 0 60rpx rgba(255, 107, 107, 0.2);
}

.qr-image {
  width: 360rpx;
  height: 360rpx;
}

.qr-hint-row {
  text-align: center;
}

.qr-hint {
  font-size: 26rpx;
  color: rgba(255, 255, 255, 0.6);
}

.qr-placeholder {
  width: 100%;
  height: 320rpx;
  display: flex;
  align-items: center;
  justify-content: center;
}

.qr-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 20rpx;
}

.loading-ring {
  width: 80rpx;
  height: 80rpx;
  border: 4rpx solid rgba(255, 255, 255, 0.1);
  border-top-color: #ff6b6b;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.loading-text {
  font-size: 26rpx;
  color: rgba(255, 255, 255, 0.5);
}

.result-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16rpx;
  padding: 32rpx;
  border-radius: 24rpx;
  width: 100%;
}

.result-section.success {
  background: rgba(16, 185, 129, 0.1);
  border: 1rpx solid rgba(16, 185, 129, 0.3);
}

.result-section.error {
  background: rgba(239, 68, 68, 0.1);
  border: 1rpx solid rgba(239, 68, 68, 0.3);
}

.result-icon { font-size: 60rpx; }
.result-text { font-size: 28rpx; color: #fff; text-align: center; }

.btn-retry {
  padding: 20rpx 48rpx;
  border: none;
  border-radius: 16rpx;
  background: rgba(255, 107, 107, 0.2);
  color: #ff6b6b;
  font-size: 26rpx;
  margin-top: 8rpx;
}

.log-scroll {
  width: 100%;
  max-height: 280rpx;
  background: rgba(0, 0, 0, 0.4);
  border-radius: 16rpx;
  padding: 20rpx;
  border: 1rpx solid rgba(255, 255, 255, 0.06);
}

.log-content {
  display: flex;
  flex-direction: column;
  gap: 6rpx;
}

.log-line {
  font-size: 22rpx;
  color: rgba(0, 255, 0, 0.85);
  font-family: monospace;
  line-height: 1.6;
}

.btn-close {
  width: 100%;
  padding: 30rpx;
  border: none;
  border-radius: 20rpx;
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.65);
  font-size: 28rpx;
  margin-bottom: 60rpx;
}

.btn-close:active { opacity: 0.7; }
</style>
