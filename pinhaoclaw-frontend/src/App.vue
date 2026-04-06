<script setup lang="ts">
</script>

<template>
  <page />
</template>

<style>
/* 全局样式 */
page {
  background-color: #f5f7fa;
  color: #303133;
  font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Segoe UI",
    "Helvetica Neue", sans-serif;
  min-height: 100vh;
}

* {
  box-sizing: border-box;
}

/* H5 桌面端优化 */
/* #ifdef H5 */
body {
  margin: 0;
  padding: 0;
  background: #f0f2f5;
}
/* #endif */
</style>
