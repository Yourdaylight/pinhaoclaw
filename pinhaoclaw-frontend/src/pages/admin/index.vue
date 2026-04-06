<template>
  <!-- #ifdef H5 -->
  <div class="h5-admin">
    <!-- 路径验证未通过 → 显示 404 -->
    <div v-if="gateStatus === 'fail'" class="gate-404">
      <el-result icon="warning" title="404" sub-title="页面不存在">
        <template #extra>
          <el-button type="primary" @click="$router.push('/')">返回首页</el-button>
        </template>
      </el-result>
    </div>

    <!-- 路径验证通过但未登录：登录弹窗 -->
    <el-dialog v-if="gateStatus === 'ok'" v-model="loginVisible" title="🦞 虾主验证" width="400px" :close-on-click-modal="false" :show-close="false">
      <el-form @submit.prevent="doAdminLogin">
        <el-form-item label="管理密码">
          <el-input
            v-model="adminPwd"
            type="password"
            placeholder="请输入管理密码"
            show-password
            size="large"
            :prefix-icon="LockIcon"
            @keyup.enter="doAdminLogin"
          />
        </el-form-item>
        <el-alert v-if="loginErr" :title="loginErr" type="error" :closable="false" style="margin-bottom: 12px" />
        <el-button type="primary" size="large" style="width: 100%" @click="doAdminLogin">进入管理台</el-button>
      </el-form>
    </el-dialog>

    <!-- 已登录：管理面板 -->
    <el-container v-if="isAuthed" class="admin-layout">
      <!-- 左侧导航 -->
      <el-aside width="220px" class="admin-sidebar">
        <div class="sidebar-header">
          <span class="sidebar-logo">🦞</span>
          <span class="sidebar-brand">拼好虾</span>
          <span class="sidebar-sub">管理控制台</span>
        </div>

        <el-menu
          :default-active="activeTab"
          class="admin-menu"
          background-color="#1a1a2e"
          text-color="#a0aec0"
          active-text-color="#fff"
          @select="(key: string) => { activeTab = key }"
        >
          <el-menu-item index="nodes">
            <el-icon><Monitor /></el-icon> 节点管理
          </el-menu-item>
          <el-menu-item index="invites">
            <el-icon><Ticket /></el-icon> 邀请码
          </el-menu-item>
          <el-menu-item index="lobsters">
            <el-icon><Coral /></el-icon> 龙虾管理
          </el-menu-item>
          <el-menu-item index="settings">
            <el-icon><Setting /></el-icon> 系统设置
          </el-menu-item>
        </el-menu>

        <div class="sidebar-footer">
          <el-button text size="small" @click="$router.push('/')">返回首页</el-button>
          <el-button text size="small" type="danger" @click="userStore.logout(); isAuthed=false">退出</el-button>
        </div>
      </el-aside>

      <!-- 右侧内容 -->
      <el-container>
        <el-header class="admin-topbar" height="56px">
          <div class="topbar-left">
            <h3>{{ tabLabel }}</h3>
          </div>
          <div class="topbar-right">
            <el-button :icon="RefreshRight" circle size="small" @click="loadAll" />
          </div>
        </el-header>

        <el-main class="admin-content">
          <!-- 统计卡片 -->
          <el-row :gutter="16" class="stats-row">
            <el-col :xs="12" :sm="6">
              <el-card shadow="never" class="stat-card">
                <el-statistic title="用户数" :value="overview.total_users" />
              </el-card>
            </el-col>
            <el-col :xs="12" :sm="6">
              <el-card shadow="never" class="stat-card">
                <el-statistic title="龙虾总数" :value="overview.total_lobsters" />
              </el-card>
            </el-col>
            <el-col :xs="12" :sm="6">
              <el-card shadow="never" class="stat-card">
                <el-statistic title="运行中" :value="overview.running_lobsters" value-color="#67c23a" />
              </el-card>
            </el-col>
            <el-col :xs="12" :sm="6">
              <el-card shadow="never" class="stat-card">
                <el-statistic title="节点数" :value="overview.total_nodes" />
              </el-card>
            </el-col>
          </el-row>

          <!-- ── 节点管理 ── -->
          <div v-show="activeTab === 'nodes'">
            <el-card shadow="never">
              <template #header>
                <div style="display: flex; justify-content: space-between; align-items: center">
                  <span><strong>节点列表</strong></span>
                  <el-button type="primary" size="small" @click="addNodeVisible = true">+ 添加节点</el-button>
                </div>
              </template>

              <el-table :data="nodes" stripe style="width: 100%">
                <el-table-column prop="name" label="名称" min-width="100" />
                <el-table-column prop="region" label="区域" width="90">
                  <template #default="{ row }">{{ regionEmoji(row.region) }} {{ row.region }}</template>
                </el-table-column>
                <el-table-column prop="host" label="地址" min-width="140" font-mono />
                <el-table-column prop="status" label="状态" width="80">
                  <template #default="{ row }">
                    <el-tag :type="row.status === 'online' ? 'success' : 'info'" size="small" effect="dark">
                      {{ row.status }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="龙虾负载" width="100">
                  <template #default="{ row }">{{ row.current_count }} / {{ row.max_lobsters }}</template>
                </el-table-column>
                <el-table-column label="操作" width="160">
                  <template #default="{ row }">
                    <el-button size="small" @click="testNode(row.id)">测试连接</el-button>
                    <el-popconfirm title="确认删除此节点？" @confirm="deleteNode(row.id)">
                      <template #reference>
                        <el-button size="small" type="danger" plain>删除</el-button>
                      </template>
                    </el-popconfirm>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>
          </div>

          <!-- ── 邀请码 ── -->
          <div v-show="activeTab === 'invites'">
            <el-card shadow="never">
              <template #header>
                <div style="display: flex; justify-content: space-between; align-items: center">
                  <span><strong>邀请码列表</strong></span>
                  <el-button type="success" size="small" @click="createInvite">生成邀请码</el-button>
                </div>
              </template>

              <el-table :data="inviteList" stripe style="width: 100%">
                <el-table-column prop="code" label="邀请码" width="200">
                  <template #default="{ row }"><code>{{ row.code }}</code></template>
                </el-table-column>
                <el-table-column label="使用情况" width="140">
                  <template #default="{ row }">
                    {{ row.used_count }} / {{ row.max_uses }}
                    <el-progress
                      v-if="row.max_uses > 0"
                      :percentage="Math.round((row.used_count / row.max_uses) * 100)"
                      :stroke-width="4"
                      style="margin-top: 4px; max-width: 80px"
                    />
                  </template>
                </el-table-column>
                <el-table-column prop="created_by" label="创建者" width="120" />
                <el-table-column label="操作" width="180">
                  <template #default="{ row }">
                    <el-button size="small" @click="copyInviteLink(row.code)">复制链接</el-button>
                    <el-popconfirm title="确认删除？" @confirm="deleteInvite(row.code)">
                      <template #reference>
                        <el-button size="small" type="danger" plain>删除</el-button>
                      </template>
                    </el-popconfirm>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>
          </div>

          <!-- ── 龙虾管理 ── -->
          <div v-show="activeTab === 'lobsters'">
            <el-card shadow="never">
              <template #header><span><strong>全部龙虾</strong></span></template>
              <el-table :data="allLobsters" stripe style="width: 100%">
                <el-table-column prop="name" label="名称" min-width="120" />
                <el-table-column prop="status" label="状态" width="90">
                  <template #default="{ row }">
                    <el-tag :type="row.status === 'running' ? 'success' : 'info'" size="small" effect="dark">
                      {{ row.status }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="微信绑定" width="90">
                  <template #default="{ row }">
                    <el-tag :type="row.weixin_bound ? 'success' : 'info'" size="small">
                      {{ row.weixin_bound ? "已绑" : "未绑" }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="node_id" label="节点 ID" width="240" font-mono />
              </el-table>
            </el-card>
          </div>

          <!-- ── 系统设置 ── -->
          <div v-show="activeTab === 'settings'">
            <el-card shadow="never" style="max-width: 600px">
              <template #header><span><strong>系统配置</strong></span></template>
              <el-form :model="settingsForm" label-width="220px">
                <el-form-item label="默认 Token 配额（万/月）">
                  <el-input-number v-model="settingsForm.tokenLimitW" :min="1" :max="9999" />
                </el-form-item>
                <el-form-item label="默认空间配额（MB/月）">
                  <el-input-number v-model="settingsForm.spaceLimitMB" :min="128" :max="102400" :step="256" />
                </el-form-item>
                <el-form-item label="每用户最大龙虾数">
                  <el-input-number v-model="settingsForm.maxLobsters" :min="1" :max="20" />
                </el-form-item>
                <el-form-item>
                  <el-button type="primary" @click="saveSettings">保存设置</el-button>
                </el-form-item>
              </el-form>
            </el-card>
          </div>
        </el-main>
      </el-container>
    </el-container>

    <!-- 添加节点对话框 -->
    <el-dialog v-model="addNodeVisible" title="添加节点" width="480px" destroy-on-close>
      <el-form :model="newNode" label-width="80px">
        <el-form-item label="名称">
          <el-input v-model="newNode.name" placeholder="如：华南-广州-01" />
        </el-form-item>
        <el-form-item label="IP 地址">
          <el-input v-model="newNode.host" placeholder="192.168.x.x 或 domain.com" />
        </el-form-item>
        <el-form-item label="SSH 密码">
          <el-input v-model="newNode.ssh_password" type="password" show-password placeholder="root 密码" />
        </el-form-item>
        <el-form-item label="区域">
          <el-select v-model="newNode.region" style="width: 100%">
            <el-option label="☀️ 华南" value="华南" />
            <el-option label="❄️ 华北" value="华北" />
            <el-option label="🌤️ 华中" value="华中" />
            <el-option label="🌊 华东" value="华东" />
            <el-option label="🌐 境外" value="境外" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addNodeVisible = false">取消</el-button>
        <el-button type="primary" :loading="nodeAdding" @click="doAddNode">确认添加</el-button>
      </template>
    </el-dialog>
  </div>
  <!-- #endif -->

  <!-- #ifndef H5 -->
  <!-- 小程序端不提供管理后台，直接跳转回登录 -->
  <view class="mp-admin-placeholder">
    <text class="ph-text">管理后台仅限 Web 端访问 🦞</text>
  </view>
  <!-- #endif -->
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useUserStore } from "../../stores/user";
import {
  adminApi,
  type Node,
  type Invite,
  type Overview,
  type Settings,
} from "../../api/admin";
import type { Lobster } from "../../api/lobster";
import { http } from "../../api/request";

// #ifdef H5
import {
  Lock,
  Monitor,
  Ticket,
  Setting,
  RefreshRight,
} from "@element-plus/icons-vue";
const LockIcon = Lock;
// #endif

const userStore = useUserStore();

// ── 路径门禁状态 ──
type GateStatus = 'checking' | 'ok' | 'fail';
const gateStatus = ref<GateStatus>('checking');

// ── 登录状态 ──
const isAuthed = ref(false);
const loginVisible = ref(true);
const adminPwd = ref("");
const loginErr = ref("");

// ── Tab ──
const activeTab = ref("nodes");
const tabs = [
  { key: "nodes", label: "节点管理" },
  { key: "invites", label: "邀请码" },
  { key: "lobsters", label: "龙虾管理" },
  { key: "settings", label: "系统设置" },
];
const tabLabel = computed(
  () => tabs.find((t) => t.key === activeTab.value)?.label || ""
);

// ── 数据 ──
const overview = ref<Overview>({
  total_users: 0,
  total_lobsters: 0,
  running_lobsters: 0,
  total_nodes: 0,
});
const nodes = ref<Node[]>([]);
const invites = ref<Record<string, Invite>>({});
const allLobsters = ref<Lobster[]>([]);
const settings = ref<Settings>({
  default_monthly_token_limit: 1000000,
  default_monthly_space_limit_mb: 2048,
  default_max_lobsters_per_user: 3,
});
const settingsForm = ref({ tokenLimitW: 100, spaceLimitMB: 2048, maxLobsters: 3 });

// 添加节点
const addNodeVisible = ref(false);
const nodeAdding = ref(false);
const newNode = ref({
  name: "新节点",
  host: "",
  ssh_password: "",
  region: "华南",
});

// 邀请码列表（转数组供 el-table 使用）
const inviteList = computed(() =>
  Object.entries(invites.value).map(([code, inv]) => ({ code, ...inv }))
);

const regionEmojiMap: Record<string, string> = {
  华南: "\u2600\ufe0f",
  华北: "\u2744\ufe0f",
  华中: "\u1f324\ufe0f",
  华东: "\u1f30a",
  境外: "\ud83c\udf10",
};
function regionEmoji(r: string): string {
  return regionEmojiMap[r] || "\ud83d\udccd";
}

onMounted(async () => {
  // ── 第一步：验证管理后台隐藏路径（双重保护）──
  await verifyGate();

  if (gateStatus.value === 'fail') return; // 路径不对，显示 404

  // ── 第二步：检查是否有已存的 admin token ──
  const stored = uni.getStorageSync("pc_admin_token");
  if (stored) {
    isAuthed.value = true;
    loginVisible.value = false;
    loadAll();
  }
});

/** 验证当前 URL 路径是否匹配后端配置的 AdminPath */
async function verifyGate() {
  // #ifdef H5
  // 从 hash 路由提取路径：/#/pages/admin/index → /pages/admin/index
  const hashPath = window.location.hash.replace(/^#/, '').split('?')[0];
  // 提取第一个路径段（如 /mgr-x7Kp9qZ）
  const pathSegments = hashPath.split('/').filter(Boolean);
  const firstSegment = pathSegments[0] || ''; // "admin" 或自定义路径

  try {
    const res: any = await http.get('/api/admin/gate?path=' + firstSegment);
    if (res.ok) {
      gateStatus.value = 'ok';
      loginVisible.value = true;
    } else {
      gateStatus.value = 'fail';
    }
  } catch {
    gateStatus.value = 'fail';
  }
  // #endif

  // #ifndef H5
  // 小程序端直接放行（admin 页面本身就不在小程序编译中，但以防万一）
  gateStatus.value = 'ok';
  // #endif
}

async function doAdminLogin() {
  loginErr.value = "";
  if (!adminPwd.value) {
    loginErr.value = "请输入密码";
    return;
  }
  await adminApi.login(adminPwd.value).then((res) => {
    if (res.ok) {
      uni.setStorageSync("pc_admin_token", res.token);
      isAuthed.value = true;
      loginVisible.value = false;
      loadAll();
    } else {
      loginErr.value = res.message || "密码错误";
    }
  });
}

async function loadAll() {
  await Promise.all([
    adminApi.overview().then((d) => (overview.value = d)).catch(() => {}),
    adminApi.nodes().then((d) => (nodes.value = d)).catch(() => {}),
    adminApi.invites().then((d) => (invites.value = d)).catch(() => {}),
    adminApi.lobsters().then((d) => (allLobsters.value = d)).catch(() => {}),
    adminApi.settings().then((d) => {
      settings.value = d;
      settingsForm.value = {
        tokenLimitW: Math.round(d.default_monthly_token_limit / 10000),
        spaceLimitMB: d.default_monthly_space_limit_mb,
        maxLobsters: d.default_max_lobsters_per_user,
      };
    }).catch(() => {}),
  ]);
}

async function doAddNode() {
  if (!newNode.value.host) return;
  nodeAdding.value = true;
  await adminApi
    .addNode(newNode.value)
    .then(() => {
      addNodeVisible.value = false;
      newNode.value = { name: "新节点", host: "", ssh_password: "", region: "华南" };
      adminApi.nodes().then((d) => (nodes.value = d));
    })
    .catch(() => {});
  nodeAdding.value = false;
}

async function testNode(id: string) {
  await adminApi.testNode(id)
    .then((res) => {})
    .catch(() => {});
}

async function deleteNode(id: string) {
  await adminApi.deleteNode(id).catch(() => {});
  adminApi.nodes().then((d) => (nodes.value = d));
}

async function createInvite() {
  await adminApi.createInvite()
    .then((res) => {
      // #ifdef H5
      const url = `${window.location.origin}/#/pages/login/index?code=${res.code}`;
      navigator.clipboard?.writeText(url);
      // #endif
      adminApi.invites().then((d) => (invites.value = d));
    })
    .catch(() => {});
}

function copyInviteLink(code: string) {
  // #ifdef H5
  const url = `${window.location.origin}/#/pages/login/index?code=${code}`;
  navigator.clipboard?.writeText(url);
  // #endif
}

async function deleteInvite(code: string) {
  await adminApi.deleteInvite(code).catch(() => {});
  adminApi.invites().then((d) => (invites.value = d));
}

async function saveSettings() {
  await adminApi
    .updateSettings({
      default_monthly_token_limit:
        (Number(settingsForm.value.tokenLimitW) || 100) * 10000,
      default_monthly_space_limit_mb:
        Number(settingsForm.value.spaceLimitMB) || 2048,
      default_max_lobsters_per_user:
        Number(settingsForm.value.maxLobsters) || 3,
    })
    .then(() => {})
    .catch(() => {});
}
</script>

<style scoped>
/* #ifdef H5 */
.h5-admin {
  min-height: 100vh;
  background: #f0f2f5;
}

.gate-404 {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
}

.admin-layout {
  min-height: 100vh;
}

.admin-sidebar {
  background: #1a1a2e;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
}

.sidebar-header {
  padding: 24px 20px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  text-align: center;
}

.sidebar-logo {
  font-size: 32px;
  display: block;
}

.sidebar-brand {
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  display: block;
  margin-top: 4px;
}

.sidebar-sub {
  color: rgba(255, 255, 255, 0.35);
  font-size: 11px;
  display: block;
}

.admin-menu {
  border-right: none;
  padding: 12px 0;
}

.admin-menu .el-menu-item {
  height: 46px;
  line-height: 46px;
  margin: 2px 10px;
  border-radius: 8px;
}

.admin-footer {
  margin-top: auto;
  padding: 16px 20px;
  display: flex;
  justify-content: center;
  gap: 8px;
}

.admin-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.04);
}

.topbar-left h3 {
  margin: 0;
  font-size: 17px;
  color: #303133;
}

.admin-content {
  padding: 20px 24px;
  background: #f0f2f5;
}

.stats-row {
  margin-bottom: 20px;
}

.stat-card {
  background: #fff;
}

.stat-card :deep(.el-statistic__head) {
  font-size: 13px;
}

.stat-card :deep(.el-statistic__content) {
  font-size: 28px;
}
/* #endif */

/* #ifndef H5 */
.mp-admin-placeholder {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #0f0c29;
}
.ph-text {
  color: rgba(255, 255, 255, 0.45);
  font-size: 28rpx;
}
/* #endif */
</style>
