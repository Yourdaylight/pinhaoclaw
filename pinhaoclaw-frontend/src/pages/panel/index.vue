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
                    v-if="l.status !== 'starting'"
                    type="primary"
                    size="small"
                    @click="onBind(l.id)"
                  >{{ bindButtonLabel(l) }}</el-button>

                  <el-button
                    size="small"
                    @click="openSkillManager(l.id, l.name)"
                  >Skills</el-button>

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

    <!-- 绑定微信弹窗（H5） -->
    <el-dialog
      v-model="showBindDialog"
      title="微信绑定向导"
      width="860px"
      destroy-on-close
      append-to-body
      @close="closeBindDialog"
    >
      <div class="bind-shell">
        <div class="bind-left">
          <div class="bind-title">扫码绑定微信</div>
          <div class="bind-subtitle">请使用微信扫一扫，把微信账号绑定到这只龙虾</div>

          <div v-if="bindQrImageSrc" class="bind-qr-card">
            <img class="bind-qr-image" :src="bindQrImageSrc" alt="微信绑定二维码" />
          </div>
          <div v-else-if="!bindIsDone && !bindIsError" class="bind-qr-placeholder">
            <el-icon class="is-loading" style="font-size: 28px; margin-bottom: 10px"><Loading /></el-icon>
            <span>正在等待二维码...</span>
          </div>

          <div v-if="bindIsDone" class="bind-result bind-success">✅ 绑定成功，龙虾已上线</div>
          <div v-if="bindIsError" class="bind-result bind-error">❌ {{ bindErrorMsg }}</div>
        </div>

        <div class="bind-right">
          <div class="bind-log-title">实时进度</div>
          <div ref="bindLogPane" class="bind-log-pane">
            <div v-for="(line, idx) in bindLogLines" :key="idx" class="bind-log-line">{{ line }}</div>
          </div>
        </div>
      </div>
      <template #footer>
        <el-button v-if="bindIsError" type="warning" @click="retryBind">重试绑定</el-button>
        <el-button @click="showBindDialog = false">关闭</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="showSkillDialog"
      title="Skill 管理"
      width="900px"
      destroy-on-close
    >
      <div class="skill-dialog-header">
        <div>
          <div class="skill-dialog-title">{{ skillTargetName || '当前龙虾' }}</div>
          <div class="skill-dialog-subtitle">从 Skill 库快速选择并下发到这只龙虾</div>
        </div>
        <el-button size="small" :loading="skillLoading" @click="loadSkillManagerData">刷新</el-button>
      </div>

      <div v-if="skillLoading" class="skill-loading-area">
        <el-skeleton :rows="5" animated />
      </div>
      <div v-else class="skill-grid">
        <div v-for="skill in skillLibrary" :key="skill.slug" class="skill-card">
          <div class="skill-card-head">
            <div>
              <div class="skill-card-title">{{ skill.display_name || skill.slug }}</div>
              <div class="skill-card-slug">{{ skill.slug }}</div>
            </div>
            <el-tag size="small" :type="skillInstalled(skill.slug) ? 'success' : 'info'">
              {{ skillInstalled(skill.slug) ? '已安装' : '未安装' }}
            </el-tag>
          </div>
          <div class="skill-card-summary">{{ skill.summary || '暂无摘要' }}</div>
          <div class="skill-card-meta">
            <span>版本 {{ skill.version || '-' }}</span>
            <span>{{ skill.category || '未分类' }}</span>
            <span>{{ skill.source?.type || '-' }}</span>
          </div>
          <div v-if="skill.requires?.bins?.length || skill.requires?.env?.length" class="skill-card-reqs">
            <span v-for="bin in skill.requires?.bins || []" :key="`${skill.slug}-bin-${bin}`" class="skill-pill">{{ bin }}</span>
            <span v-for="env in skill.requires?.env || []" :key="`${skill.slug}-env-${env}`" class="skill-pill skill-pill-env">{{ env }}</span>
          </div>
          <div class="skill-card-actions">
            <el-button
              v-if="!skillInstalled(skill.slug)"
              type="primary"
              size="small"
              :loading="skillActionKey === `install:${skill.slug}`"
              @click="installSkillToTarget(skill.slug)"
            >安装到龙虾</el-button>
            <el-button
              v-else
              size="small"
              type="danger"
              plain
              :loading="skillActionKey === `remove:${skill.slug}`"
              @click="uninstallSkillFromTarget(skill.slug)"
            >移除</el-button>
          </div>
        </div>
      </div>
      <el-empty v-if="!skillLoading && skillLibrary.length === 0" description="管理员还没有上传可用的 Skill" />
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
import { ref, onMounted, onUnmounted, nextTick } from "vue";
import { useUserStore } from "../../stores/user";
import { useLobsterStore } from "../../stores/lobster";
import { authApi } from "../../api/auth";
import { getBaseUrl } from "../../api/request";
import { lobsterApi, type SkillRegistryEntry, type LobsterSkillInfo } from "../../api/lobster";
// #ifdef H5
import { Loading, Plus, UserFilled } from "@element-plus/icons-vue";
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
const showBindDialog = ref(false);
const bindTargetId = ref("");
const bindQrImageSrc = ref("");
const bindLogLines = ref<string[]>([]);
const bindIsDone = ref(false);
const bindIsError = ref(false);
const bindErrorMsg = ref("");
const bindLogPane = ref<HTMLElement | null>(null);
const showSkillDialog = ref(false);
const skillTargetId = ref("");
const skillTargetName = ref("");
const skillLoading = ref(false);
const skillActionKey = ref("");
const skillLibrary = ref<SkillRegistryEntry[]>([]);
const installedSkills = ref<LobsterSkillInfo[]>([]);

// #ifdef H5
let bindEventSource: EventSource | null = null;
// #endif

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

onUnmounted(() => {
  cleanupBindStream();
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
  // #ifdef H5
  bindTargetId.value = id;
  resetBindState();
  showBindDialog.value = true;
  startBindFlow();
  // #endif

  // #ifndef H5
  uni.navigateTo({ url: `/pages/lobster/bind?id=${id}` });
  // #endif
}

async function openSkillManager(id: string, name?: string) {
  skillTargetId.value = id;
  skillTargetName.value = name || "";
  showSkillDialog.value = true;
  await loadSkillManagerData();
}

async function loadSkillManagerData() {
  if (!skillTargetId.value) return;
  skillLoading.value = true;
  try {
    const [libraryRes, installedRes] = await Promise.all([
      lobsterApi.skillLibrary(),
      lobsterApi.listSkills(skillTargetId.value),
    ]);
    skillLibrary.value = libraryRes.skills || [];
    installedSkills.value = installedRes.skills || [];
  } catch (err: any) {
    // #ifdef H5
    ElMessage.error(err?.message || "加载 Skill 列表失败");
    // #endif
    // #ifndef H5
    uni.showToast({ title: err?.message || "加载 Skill 列表失败", icon: "error" });
    // #endif
  } finally {
    skillLoading.value = false;
  }
}

function skillInstalled(slug: string) {
  return installedSkills.value.some((item) => item.slug === slug);
}

async function installSkillToTarget(slug: string) {
  if (!skillTargetId.value) return;
  skillActionKey.value = `install:${slug}`;
  try {
    const res = await lobsterApi.installSkill(skillTargetId.value, slug);
    // #ifdef H5
    ElMessage.success(res.message || "Skill 安装成功");
    // #endif
    // #ifndef H5
    uni.showToast({ title: res.message || "Skill 安装成功", icon: "success" });
    // #endif
    await loadSkillManagerData();
  } catch (err: any) {
    // #ifdef H5
    ElMessage.error(err?.message || "Skill 安装失败");
    // #endif
    // #ifndef H5
    uni.showToast({ title: err?.message || "Skill 安装失败", icon: "error" });
    // #endif
  } finally {
    skillActionKey.value = "";
  }
}

async function uninstallSkillFromTarget(slug: string) {
  if (!skillTargetId.value) return;
  skillActionKey.value = `remove:${slug}`;
  try {
    const res = await lobsterApi.uninstallSkill(skillTargetId.value, slug);
    // #ifdef H5
    ElMessage.success(res.message || "Skill 已移除");
    // #endif
    // #ifndef H5
    uni.showToast({ title: res.message || "Skill 已移除", icon: "success" });
    // #endif
    await loadSkillManagerData();
  } catch (err: any) {
    // #ifdef H5
    ElMessage.error(err?.message || "移除 Skill 失败");
    // #endif
    // #ifndef H5
    uni.showToast({ title: err?.message || "移除 Skill 失败", icon: "error" });
    // #endif
  } finally {
    skillActionKey.value = "";
  }
}

function resetBindState() {
  bindQrImageSrc.value = "";
  bindLogLines.value = ["连接中..."];
  bindIsDone.value = false;
  bindIsError.value = false;
  bindErrorMsg.value = "";
}

function addBindLog(message: string) {
  if (!message) return;
  bindLogLines.value.push(message);
  nextTick(() => {
    if (bindLogPane.value) {
      bindLogPane.value.scrollTop = bindLogPane.value.scrollHeight;
    }
  });
}

function parseBindPayload<T>(raw: string, fallback: T): T {
  try {
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

function cleanupBindStream() {
  // #ifdef H5
  if (bindEventSource) {
    bindEventSource.close();
    bindEventSource = null;
  }
  // #endif
}

function closeBindDialog() {
  cleanupBindStream();
  bindTargetId.value = "";
}

function handleBindEvent(event: string, stage: string, message: string, url?: string) {
  if (event === "qrcode" && url) {
    const base = getBaseUrl();
    bindQrImageSrc.value = `${base}/api/qrcode?url=${encodeURIComponent(url)}`;
    addBindLog("二维码已生成，请用微信扫一扫");
    return;
  }

  if (event === "done") {
    bindIsDone.value = true;
    addBindLog(message || "绑定完成");
    lobsterStore.fetchAll().catch(() => {});
    return;
  }

  if (event === "error") {
    bindIsError.value = true;
    bindErrorMsg.value = message || "绑定失败";
    addBindLog(`错误: ${bindErrorMsg.value}`);
    return;
  }

  if (message) {
    const prefix = stage === "waiting" ? "等待扫码: " : "";
    addBindLog(prefix + message);
  }
}

function startBindFlow() {
  // #ifdef H5
  cleanupBindStream();
  if (!bindTargetId.value) {
    bindIsError.value = true;
    bindErrorMsg.value = "龙虾 ID 无效";
    return;
  }
  const token = userStore.token;
  if (!token) {
    bindIsError.value = true;
    bindErrorMsg.value = "登录已失效，请重新登录";
    return;
  }

  const base = getBaseUrl();
  const sseUrl = `${base}/api/lobsters/${encodeURIComponent(bindTargetId.value)}/bind?token=${encodeURIComponent(token)}`;
  const es = new EventSource(sseUrl);
  bindEventSource = es;

  es.addEventListener("progress", (e: MessageEvent) => {
    const d = parseBindPayload(e.data, { stage: "", message: "" });
    handleBindEvent("progress", d.stage, d.message);
  });

  es.addEventListener("qrcode", (e: MessageEvent) => {
    const d = parseBindPayload(e.data, { stage: "", message: "", url: "" });
    handleBindEvent("qrcode", d.stage, d.message, d.url);
  });

  es.addEventListener("done", (e: MessageEvent) => {
    const d = parseBindPayload(e.data, { stage: "", message: "" });
    handleBindEvent("done", d.stage, d.message);
    cleanupBindStream();
  });

  es.onerror = () => {
    if (!bindIsDone.value && !bindIsError.value) {
      handleBindEvent("error", "error", "连接断开，请重试");
    }
    cleanupBindStream();
  };
  // #endif
}

function retryBind() {
  resetBindState();
  startBindFlow();
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
  const map: Record<string, any> = {
    created: "info",
    binding: "warning",
    running: "success",
    stopped: "info",
    error: "danger",
  };
  return map[status] || "info";
}

function statusLabel(s: string): string {
  const map: Record<string, string> = {
    created: "待绑定",
    binding: "绑定中（可重试）",
    running: "运行中",
    stopped: "已停止",
    error: "绑定失败",
  };
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

function bindButtonLabel(l: any): string {
  if (l.status === "binding" || l.status === "error") return "重试绑定";
  return l.weixin_bound ? "微信管理" : "绑定微信";
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

.bind-shell {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  min-height: 460px;
}

.bind-left {
  border: 1px solid #ebeef5;
  border-radius: 14px;
  padding: 16px;
  background: linear-gradient(160deg, #f8fbff 0%, #f5f9f2 100%);
}

.bind-title {
  font-size: 17px;
  font-weight: 700;
  color: #1f2937;
}

.bind-subtitle {
  margin-top: 6px;
  color: #6b7280;
  font-size: 13px;
  line-height: 1.5;
}

.bind-qr-card {
  margin-top: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 286px;
  background: #fff;
  border: 1px solid #d1d5db;
  border-radius: 12px;
}

.bind-qr-image {
  width: 256px;
  height: 256px;
  object-fit: contain;
}

.bind-qr-placeholder {
  margin-top: 16px;
  min-height: 286px;
  border: 1px dashed #9ca3af;
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #6b7280;
}

.bind-right {
  border: 1px solid #ebeef5;
  border-radius: 14px;
  padding: 12px;
  background: #0b1220;
  color: #d1d5db;
}

.bind-log-title {
  font-size: 13px;
  color: #93c5fd;
  margin-bottom: 8px;
  font-weight: 600;
}

.bind-log-pane {
  height: 420px;
  overflow: auto;
  padding: 10px;
  border-radius: 10px;
  background: rgba(2, 6, 23, 0.6);
  border: 1px solid rgba(148, 163, 184, 0.25);
}

.bind-log-line {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 12px;
  color: #e2e8f0;
  line-height: 1.6;
  margin-bottom: 6px;
  word-break: break-word;
}

.bind-result {
  margin-top: 12px;
  padding: 8px 10px;
  border-radius: 8px;
  font-size: 13px;
}

.bind-success {
  background: #ecfdf5;
  border: 1px solid #86efac;
  color: #166534;
}

.bind-error {
  background: #fef2f2;
  border: 1px solid #fca5a5;
  color: #991b1b;
}

.skill-dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.skill-dialog-title {
  font-size: 16px;
  font-weight: 700;
  color: #111827;
}

.skill-dialog-subtitle {
  margin-top: 4px;
  font-size: 13px;
  color: #6b7280;
}

.skill-loading-area {
  padding: 12px 0;
}

.skill-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.skill-card {
  border: 1px solid #e5e7eb;
  border-radius: 14px;
  padding: 16px;
  background: linear-gradient(180deg, #ffffff 0%, #f8fafc 100%);
}

.skill-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.skill-card-title {
  font-size: 15px;
  font-weight: 700;
  color: #111827;
}

.skill-card-slug {
  margin-top: 4px;
  font-size: 12px;
  color: #6b7280;
}

.skill-card-summary {
  margin-top: 12px;
  min-height: 40px;
  font-size: 13px;
  line-height: 1.6;
  color: #4b5563;
}

.skill-card-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
  font-size: 12px;
  color: #6b7280;
}

.skill-card-reqs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}

.skill-pill {
  display: inline-flex;
  align-items: center;
  padding: 4px 8px;
  border-radius: 999px;
  background: #eef2ff;
  color: #3730a3;
  font-size: 12px;
}

.skill-pill-env {
  background: #ecfeff;
  color: #155e75;
}

.skill-card-actions {
  margin-top: 14px;
}

@media (max-width: 900px) {
  .bind-shell {
    grid-template-columns: 1fr;
  }

  .bind-log-pane {
    height: 240px;
  }

  .skill-grid {
    grid-template-columns: 1fr;
  }
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
