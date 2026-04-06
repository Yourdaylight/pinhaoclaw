<template>
  <view class="create-page">
    <text class="title">购买龙虾</text>
    <text class="desc">此功能请从面板页购买</text>
  </view>
</template>

<script setup lang="ts">
import { onMounted } from "vue";
onMounted(() => {
  uni.navigateBack();
});
</script>

<style scoped>
.create-page {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: #0f0c29;
  color: #fff;
  gap: 20rpx;
}
</style>
