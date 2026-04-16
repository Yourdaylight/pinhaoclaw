<template>
  <!-- #ifdef H5 -->
  <div class="h5-panel">
    <el-container class="panel-layout">
      <!-- 顶栏 -->
      <el-header class="panel-header" height="56px">
        <div class="header-left">
          <span class="brand-emoji">🦞</span>
          <span class="brand-text">拼好虾 PinHaoClaw</span>
        </div>
        <div class="header-right">
          <el-dropdown trigger="click">
            <span class="user-info">
              <el-avatar :size="28" icon="UserFilled" />
              {{ userStore.userName || "用户" }}
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="userStore.logout()">退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <!-- 主内容 -->
      <el-main class="panel-main">
        <!-- 操作栏 -->
        <div class="action-bar">
          <h2>我的龙虾</h2>
          <el-button type="primary" @click="openCreate">
            <el-icon><Plus /></el-icon> 购买龙虾
          </el-button>
        </div>

        <!-- 龙虾列表（桌面用卡片网格） -->
        <div v-if="lobsterStore.loading && lobsterStore.lobsters.length === 0" class="loading-area">
          <el-skeleton :rows="4" animated />
          <el-skeleton :rows="4" animated style="margin-top: 16px" />
        </div>

        <el-row v-if="!lobsterStore.loading" :gutter="20">
          <el-col
            v-for="l in lobsterStore.lobsters"
            :key="l.id"
            :xs="24"
            :sm="12"
            :lg="8"
            style="margin-bottom: 20px"
          >
            <el-card shadow="hover" class="lobster-card">
              <template #header>
                <div class="card-head">
                  <span class="card-name">{{ l.name }}</span>
                  <el-tag
                    :type="statusType(l.status)"
                    size="small"
                    effect="dark"
                    round
                  >
                    {{ statusLabel(l.status) }}
                  </el-tag>
                </div>
              </template>

              <div class="card-body">
                <div class="info-row">
                  <span class="label">区域：</span>
                  <span>{{ l.region || "自动选择" }}</span>
                </div>
                <div class="info-row">
                  <span class="label">微信绑定：</span>
                  <el-tag :type="l.weixin_bound ? 'success' : 'info'" size="small">
                    {{ l.weixin_bound ? "已绑定 ✅" : "未绑定" }}
                  </el-tag>
                </div>
                <div class="info-row">
                  <span class="label">创建时间：</span>
                  <span>{{ formatTime(l.created_at) }}</span>
                </div>

                <el-divider style="margin: 12px 0" />

                <div class="quota-section">
                  <div class="quota-label">用量配额</div>
                  <el-progress
                    :percentage="calcQuota(l)"
                    :status="quotaStatus(l)"
                    :stroke-width="10"
                  />
                </div>

                <div class="card-actions">
                  <el-button
                    v-if="!l.weixin_bound && (l.status === 'running' || l.status === 'created')"
                    type="primary"
                    size="small"
                    @click="onBind(l.id)"
                  >绑定微信</el-button>

                  <el-button
                    v-if="l.status === 'running'"
                    size="small"
                    @click="onStop(l.id)"
                  >停止</el-button>

                  <el-button
                    v-if="l.status === 'stopped'"
                    type="warning"
                    size="small"
                    @click="onStart(l.id)"
                  >启动</el-button>

                  <el-popconfirm
                    title="确定释放这只龙虾？"
                    confirm-button-text="释放"
                    cancel-button-text="取消"
                    confirm-button-type="danger"
                    @confirm="onDelete(l.id, l.name)"
                  >
                    <template #reference>
                      <el-button type="danger" size="small" plain>释放</el-button>
                    </template>
                  </el-popconfirm>
                </div>
              </div>
            </el-card>
          </el-col>
        </el-row>

        <!-- 空状态 -->
        <el-empty
          v-if="!lobsterStore.loading && lobsterStore.lobsters.length === 0"
          description="你还没有龙虾"
          :image-size="120"
        >
          <el-button type="primary" @click="openCreate">购买我的第一只龙虾 🦞</el-button>
        </el-empty>
      </el-main>
    </el-container>

    <!-- 购买弹窗 -->
    <el-dialog v-model="showCreate" title="🦞 购买新龙虾" width="480px" destroy-on-close>
      <p style="color: #909399; font-size: 13px; margin-bottom: 16px">
        给龙虾起个名字，选择区域，它将成为你的专属 AI 助手。
      </p>

      <el-form label-width="80px">
        <el-form-item label="名称">
          <el-input
            v-model="newName"
            placeholder="如：小红、工作助手"
            clearable
          />
        </el-form-item>
        <el-form-item label="区域">
          <el-select v-model="regionIndex" placeholder="选择区域" style="width: 100%">
            <el-option
              v-for="(r, idx) in regionOptions"
              :key="idx"
              :label="r"
              :value="idx"
            />
          </el-select>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="doCreate">
          确认购买
        </el-button>
      </template>
    </el-dialog>
  </div>
  <!-- #endif -->

  <!-- #ifndef H5 -->
  <view class="mp-panel">
    <view class="header">
      <view class="header-left">
        <text class="header-emoji">🦞</text>
        <text class="header-title">我的龙虾</text>
      </view>
      <view class="header-right">
        <text class="user-name">{{ userStore.userName }}</text>
        <text class="logout-btn" @click="userStore.logout()">退出</text>
      </view>
    </view>

    <scroll-view
      class="scroll-area"
      scroll-y
      refresher-enabled
      :refresher-triggered="refreshing"
      @refresherrefresh="onRefresh"
    >
      <view class="list-content">
        <view v-if="!lobsterStore.loading && lobsterStore.lobsters.length === 0" class="empty-state">
          <text class="empty-emoji">🦞</text>
          <text class="empty-title">你还没有龙虾</text>
          <button class="btn-buy-first" @click="openCreate">+ 购买我的第一只龙虾</button>
        </view>

        <LobsterCard
          v-for="l in lobsterStore.lobsters"
          :key="l.id"
          :lobster="l"
          @bind="onBind"
          @stop="onStop"
          @start="onStart"
          @delete="onDelete"
        />

        <view v-if="lobsterStore.lobsters.length > 0" class="add-more">
          <button class="btn-add" @click="openCreate">+ 购买新龙虾</button>
        </view>
        <view class="safe-area" />
      </view>
    </scroll-view>

    <view class="modal-overlay" v-if="showCreate" @click.self="showCreate = false">
      <view class="modal">
        <text class="modal-title">🦞 购买新龙虾</text>
        <input class="modal-input" v-model="newName" placeholder="龙虾的名字" placeholder-class="input-ph" />
        <picker
          :range="regionOptions"
          :value="regionIndex"
          @change="regionIndex = $event.detail.value"
        >
          <view class="picker-view">
            <text>{{ regionOptions[regionIndex] }}</text>
            <text class="picker-arrow">›</text>
          </view>
        </picker>
        <button class="btn-confirm" :disabled="creating" @click="doCreate">
          {{ creating ? "创建中..." : "确认购买" }}
        </button>
        <button class="btn-cancel" @click="showCreate = false">取消</button>
      </view>
    </view>
  </view>
  <!-- #endif -->
</template>

<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useUserStore } from "../../stores/user";
import { useLobsterStore } from "../../stores/lobster";
import { authApi } from "../../api/auth";
// #ifdef H5
import { Plus, UserFilled } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
// #endif
// #ifndef H5
import LobsterCard from "../../components/LobsterCard.vue";
// #endif

const userStore = useUserStore();
const lobsterStore = useLobsterStore();

const refreshing = ref(false);
const showCreate = ref(false);
const newName = ref("");
const creating = ref(false);
const regionOptions = ref(["🌐 自动选择最优节点"]);
const regionIndex = ref(0);
const regions = ref<string[]>([]);

const regionEmojiMap: Record<string, string> = {
  华南: "☀️", 华北: "❄️", 华中: "🌤️", 华东: "🌊", 境外: "🌐",
};

onMounted(async () => {
  if (!userStore.isLoggedIn) {
    uni.reLaunch({ url: "/pages/login/index" });
    return;
  }
  // Fetch user profile (fills userName etc.) if not yet populated
  if (!userStore.userName) {
    userStore.fetchMe().catch(() => {});
  }
  uni.showLoading({ title: "加载中..." });
  await lobsterStore.fetchAll();
  uni.hideLoading();
});

async function onRefresh() {
  refreshing.value = true;
  await lobsterStore.fetchAll();
  refreshing.value = false;
}

async function openCreate() {
  showCreate.value = true;
  newName.value = "";
  regionIndex.value = 0;
  authApi.regions().then((res) => {
    regions.value = res.regions || [];
    regionOptions.value = [
      "🌐 自动选择最优节点",
      ...regions.value.map((r) => `${regionEmojiMap[r] || "📍"} ${r}`),
    ];
  }).catch(() => {});
}

async function doCreate() {
  const name = newName.value.trim();
  const region = regionIndex.value > 0 ? regions.value[regionIndex.value - 1] : undefined;
  creating.value = true;
  await lobsterStore.create(name || `龙虾${lobsterStore.lobsters.length + 1}号`, region)
    .then(() => {
      showCreate.value = false;
      // #ifdef H5
      ElMessage.success("龙虾已创建 🦞");
      // #endif
      // #ifndef H5
      uni.showToast({ title: "龙虾已创建 🦞", icon: "success" });
      // #endif
    })
    .catch((err: Error) => {
      // #ifndef H5
      uni.showToast({ title: err.message || "创建失败", icon: "error" });
      // #endif
    });
  creating.value = false;
}

async function onBind(id: string) {
  uni.navigateTo({ url: `/pages/lobster/bind?id=${id}` });
}

async function onStop(id: string) {
  try {
    await lobsterStore.stop(id);
    // #ifdef H5
    ElMessage.success("龙虾已停止");
    // #endif
    // #ifndef H5
    uni.showToast({ title: "龙虾已停止", icon: "success" });
    // #endif
  } catch {
    // #ifdef H5
    ElMessage.error("停止失败");
    // #endif
    // #ifndef H5
    uni.showToast({ title: "停止失败", icon: "error" });
    // #endif
  }
}

async function onStart(id: string) {
  try {
    await lobsterStore.start(id);
    // #ifdef H5
    ElMessage.success("龙虾已启动");
    // #endif
    // #ifndef H5
    uni.showToast({ title: "龙虾已启动", icon: "success" });
    // #endif
  } catch {
    // #ifdef H5
    ElMessage.error("启动失败");
    // #endif
    // #ifndef H5
    uni.showToast({ title: "启动失败", icon: "error" });
    // #endif
  }
}

async function onDelete(id: string, name: string) {
  try {
    await lobsterStore.remove(id);
    // #ifdef H5
    ElMessage.success("龙虾已释放");
    // #endif
    // #ifndef H5
    uni.showToast({ title: "龙虾已释放", icon: "success" });
    // #endif
  } catch {
    // #ifdef H5
    ElMessage.error("释放失败");
    // #endif
    // #ifndef H5
    uni.showToast({ title: "释放失败", icon: "error" });
    // #endif
  }
}

// ── H5 辅助函数 ──
function statusType(status: string): "" | "success" | "warning" | "danger" | "info" {
  const map: Record<string, any> = { running: "success", stopped: "info", error: "danger" };
  return map[status] || "info";
}

function statusLabel(s: string): string {
  const map: Record<string, string> = { running: "运行中", stopped: "已停止", error: "异常" };
  return map[s] || s;
}

function calcQuota(l: any): number {
  const limit = l.monthly_token_limit;
  if (!limit) return 0;
  return Math.min(100, Math.round((l.monthly_token_used / limit) * 100));
}

function quotaStatus(l: any): "" | "success" | "warning" | "exception" {
  const limit = l.monthly_token_limit;
  if (!limit) return "";
  const p = l.monthly_token_used / limit;
  if (p >= 0.95) return "exception";
  if (p >= 0.75) return "warning";
  return "success";
}

function formatTime(t?: string): string {
  if (!t) return "-";
  return t.replace("T", " ").slice(0, 16);
}
</script>

<style scoped>
/* ── H5 桌面端样式 ── */
/* #ifdef H5 */
.h5-panel {
  min-height: 100vh;
  background: #f0f2f5;
}

.panel-layout {
  min-height: 100vh;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  padding: 0 24px;
  box-shadow: 0 1px 4px rgba(0,0,0,0.06);
  z-index: 10;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.brand-emoji { font-size: 24px; }

.brand-text {
  font-size: 18px;
  font-weight: 700;
  color: #303133;
  letter-spacing: 0.5px;
}

.header-right .user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  color: #606266;
  font-size: 14px;
}

.panel-main {
  padding: 24px;
  max-width: 1200px;
  margin: 0 auto;
  width: 100%;
}

.action-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
}

.action-bar h2 {
  margin: 0;
  font-size: 20px;
  color: #303133;
}

.loading-area {
  max-width: 600px;
}

.lobster-card {
  height: 100%;
}

.card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-name {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.card-body {
  font-size: 13px;
}

.info-row {
  display: flex;
  gap: 6px;
  margin-bottom: 6px;
  color: #606266;
}

.info-row .label {
  color: #909399;
  white-space: nowrap;
}

.quota-label {
  font-size: 12px;
  color: #909399;
  margin-bottom: 6px;
}

.card-actions {
  display: flex;
  gap: 8px;
  margin-top: 14px;
}
/* #endif */

/* ── 小程序端样式 ── */
/* #ifndef H5 */
.mp-panel {
  min-height: 100vh;
  background: linear-gradient(180deg, #0f0c29 0%, #1a1a2e 100%);
  display: flex;
  flex-direction: column;
}

.header {
  padding: 24rpx 32rpx;
  padding-top: calc(24rpx + env(safe-area-inset-top));
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: rgba(15, 12, 41, 0.95);
  backdrop-filter: blur(10px);
  border-bottom: 1rpx solid rgba(255,255,255,0.06);
  position: sticky;
  top: 0;
  z-index: 10;
}

.header-left { display: flex; align-items: center; gap: 12rpx; }
.header-emoji { font-size: 40rpx; }
.header-title { font-size: 34rpx; font-weight: 700; color: #fff; }
.header-right { display: flex; align-items: center; gap: 24rpx; }
.user-name { font-size: 26rpx; color: rgba(255,255,255,0.55); }
.logout-btn {
  font-size: 26rpx; color: #ff6b6b; padding: 8rpx 20rpx;
  border: 1rpx solid rgba(255,107,107,0.3); border-radius: 20rpx;
}

.scroll-area { flex: 1; height: calc(100vh - 140rpx); }
.list-content { padding: 32rpx 32rpx 0; }

.empty-state {
  display: flex; flex-direction: column; align-items: center;
  padding: 120rpx 0 60rpx; gap: 20rpx;
}
.empty-emoji { font-size: 120rpx; opacity: 0.5; }
.empty-title { font-size: 32rpx; color: rgba(255,255,255,0.6); }
.btn-buy-first {
  margin-top: 20rpx; padding: 28rpx 60rpx;
  background: linear-gradient(135deg, #ff6b6b, #ff8e53);
  color: #fff; border: none; border-radius: 50rpx;
  font-size: 28rpx; font-weight: 600;
}
.add-more { margin: 8rpx 0 24rpx; }
.btn-add {
  width: 100%; padding: 28rpx;
  border: 2rpx dashed rgba(255,255,255,0.15); border-radius: 28rpx;
  background: transparent; color: rgba(255,255,255,0.4); font-size: 28rpx;
}
.safe-area { height: 60rpx; }

.modal-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,0.7);
  display: flex; align-items: flex-end; justify-content: center; z-index: 100;
}
.modal {
  width: 100%; background: #1a1a2e; border: 1rpx solid rgba(255,255,255,0.1);
  border-radius: 40rpx 40rpx 0 0;
  padding: 48rpx 40rpx calc(48rpx + env(safe-area-inset-bottom));
  display: flex; flex-direction: column; gap: 20rpx;
}
.modal-title { font-size: 36rpx; font-weight: 700; color: #fff; text-align: center; }
.modal-input {
  width: 100%; padding: 26rpx 32rpx; border: 1rpx solid rgba(255,255,255,0.15);
  border-radius: 16rpx; background: rgba(255,255,255,0.06); color: #fff; font-size: 28rpx;
}
.input-ph { color: rgba(255,255,255,0.3); }
.picker-view {
  padding: 26rpx 32rpx; border: 1rpx solid rgba(255,255,255,0.15); border-radius: 16rpx;
  background: rgba(255,255,255,0.06); display: flex; justify-content: space-between;
  align-items: center; color: #fff; font-size: 28rpx;
}
.picker-arrow { color: rgba(255,255,255,0.4); font-size: 36rpx; }
.btn-confirm {
  width: 100%; padding: 30rpx; border: none; border-radius: 20rpx;
  background: linear-gradient(135deg, #ff6b6b, #ff8e53); color: #fff;
  font-size: 30rpx; font-weight: 700; margin-top: 8rpx;
}
.btn-confirm:disabled { opacity: 0.5; }
.btn-cancel {
  width: 100%; padding: 26rpx; border: none; border-radius: 20rpx;
  background: rgba(255,255,255,0.06); color: rgba(255,255,255,0.55); font-size: 28rpx;
}
/* #endif */
</style>
