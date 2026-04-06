<template>
  <view class="quota-bar">
    <view class="quota-label">
      <text class="label-text">{{ label }}</text>
      <text class="label-value">{{ valueText }}</text>
    </view>
    <view class="bar-track">
      <view
        class="bar-fill"
        :class="fillClass"
        :style="{ width: pct + '%' }"
      />
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  label: string;
  used: number;
  limit: number;
  unit?: string;
  scale?: number; // 显示时缩放倍数，如 10000 则除以10000显示万
  scaleUnit?: string; // 缩放后单位，如 "万"
}>();

const pct = computed(() => {
  if (!props.limit) return 0;
  return Math.min(100, (props.used / props.limit) * 100);
});

const fillClass = computed(() => {
  const p = pct.value;
  if (p > 90) return "danger";
  if (p > 70) return "warn";
  return "ok";
});

const valueText = computed(() => {
  if (props.scale && props.scaleUnit) {
    const u = (props.used / props.scale).toFixed(1);
    const l = (props.limit / props.scale).toFixed(0);
    return `${u}${props.scaleUnit} / ${l}${props.scaleUnit}`;
  }
  const unit = props.unit || "";
  return `${props.used}${unit} / ${props.limit}${unit}`;
});
</script>

<style scoped>
.quota-bar {
  margin-bottom: 16rpx;
}

.quota-label {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8rpx;
}

.label-text {
  font-size: 22rpx;
  color: rgba(255, 255, 255, 0.5);
}

.label-value {
  font-size: 22rpx;
  color: rgba(255, 255, 255, 0.5);
}

.bar-track {
  height: 10rpx;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 5rpx;
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  border-radius: 5rpx;
  transition: width 0.4s ease;
}

.bar-fill.ok {
  background: linear-gradient(90deg, #10b981, #34d399);
}

.bar-fill.warn {
  background: linear-gradient(90deg, #f59e0b, #fbbf24);
}

.bar-fill.danger {
  background: linear-gradient(90deg, #ef4444, #f87171);
}
</style>
