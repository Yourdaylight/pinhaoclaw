<template>
  <view class="lobster-card" :class="{ 'is-running': lobster.status === 'running' }">
    <!-- 卡片头部 -->
    <view class="card-header">
      <view class="name-row">
        <view class="status-dot" :class="lobster.status" />
        <text class="lobster-name">{{ lobster.name }}</text>
      </view>
      <view class="tags">
        <text v-if="lobster.region" class="region-tag">{{ regionEmoji }} {{ lobster.region }}</text>
        <text class="status-text">{{ statusLabel }}</text>
      </view>
    </view>

    <!-- 微信绑定状态 -->
    <view class="weixin-status">
      <text v-if="lobster.weixin_bound" class="bound">📱 微信已绑定</text>
      <text v-else class="unbound">⚠️ 未绑定微信</text>
      <text class="node-text" v-if="lobster.node_name">{{ lobster.node_name }}</text>
    </view>

    <!-- 配额进度条 -->
    <QuotaBar
      label="Token 用量"
      :used="lobster.monthly_token_used"
      :limit="lobster.monthly_token_limit"
      :scale="10000"
      scale-unit="万"
    />
    <QuotaBar
      label="空间用量"
      :used="lobster.monthly_space_used_mb"
      :limit="lobster.monthly_space_limit_mb"
      unit="MB"
    />

    <!-- 操作按钮 -->
    <view class="card-actions">
      <button
        v-if="!lobster.weixin_bound"
        class="btn btn-primary"
        @click="$emit('bind', lobster.id)"
      >绑定微信</button>
      <button
        v-if="lobster.status === 'running'"
        class="btn btn-gray"
        @click="$emit('stop', lobster.id)"
      >休息</button>
      <button
        v-if="lobster.status === 'stopped'"
        class="btn btn-green"
        @click="$emit('start', lobster.id)"
      >唤醒</button>
      <button
        class="btn btn-danger"
        @click="$emit('delete', lobster.id, lobster.name)"
      >释放</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";
import QuotaBar from "./QuotaBar.vue";
import type { Lobster } from "../api/lobster";

const props = defineProps<{ lobster: Lobster }>();
defineEmits<{
  bind: [id: string];
  stop: [id: string];
  start: [id: string];
  delete: [id: string, name: string];
}>();

const regionEmojiMap: Record<string, string> = {
  华南: "☀️", 华北: "❄️", 华中: "🌤️", 华东: "🌊", 境外: "🌐",
};
const regionEmoji = computed(() => regionEmojiMap[props.lobster.region] || "📍");

const statusMap: Record<string, string> = {
  created: "待启动",
  running: "运行中",
  stopped: "已休息",
  binding: "绑定中",
  error: "异常",
};
const statusLabel = computed(() => statusMap[props.lobster.status] || props.lobster.status);
</script>

<style scoped>
.lobster-card {
  background: rgba(255, 255, 255, 0.06);
  border: 1rpx solid rgba(255, 255, 255, 0.1);
  border-radius: 28rpx;
  padding: 36rpx;
  margin-bottom: 24rpx;
  transition: all 0.3s;
}

.lobster-card.is-running {
  border-color: rgba(16, 185, 129, 0.25);
  box-shadow: 0 0 30rpx rgba(16, 185, 129, 0.08);
}

.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 20rpx;
}

.name-row {
  display: flex;
  align-items: center;
  gap: 14rpx;
}

.status-dot {
  width: 18rpx;
  height: 18rpx;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-dot.running {
  background: #10b981;
  box-shadow: 0 0 12rpx #10b981;
  animation: pulse 1.5s infinite;
}

.status-dot.stopped { background: #6b7280; }
.status-dot.binding { background: #f59e0b; animation: pulse 1.5s infinite; }
.status-dot.error { background: #ef4444; }
.status-dot.created { background: #3b82f6; }

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.lobster-name {
  font-size: 32rpx;
  font-weight: 600;
  color: #fff;
}

.tags {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6rpx;
}

.region-tag {
  background: rgba(255, 107, 107, 0.15);
  color: #ff8e53;
  padding: 4rpx 16rpx;
  border-radius: 8rpx;
  font-size: 22rpx;
}

.status-text {
  font-size: 22rpx;
  color: rgba(255, 255, 255, 0.4);
}

.weixin-status {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24rpx;
}

.bound { color: #10b981; font-size: 26rpx; }
.unbound { color: #f59e0b; font-size: 26rpx; }
.node-text { color: rgba(255, 255, 255, 0.3); font-size: 22rpx; }

.card-actions {
  display: flex;
  gap: 16rpx;
  flex-wrap: wrap;
  margin-top: 8rpx;
}

.btn {
  flex: 1;
  min-width: 140rpx;
  padding: 18rpx 24rpx;
  border: none;
  border-radius: 14rpx;
  font-size: 26rpx;
  font-weight: 600;
  transition: all 0.2s;
}

.btn:active { transform: scale(0.96); }

.btn-primary {
  background: linear-gradient(135deg, #ff6b6b, #ff8e53);
  color: #fff;
}

.btn-green {
  background: linear-gradient(135deg, #10b981, #34d399);
  color: #fff;
}

.btn-gray {
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.7);
}

.btn-danger {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
  border: 1rpx solid rgba(239, 68, 68, 0.3);
}
</style>
