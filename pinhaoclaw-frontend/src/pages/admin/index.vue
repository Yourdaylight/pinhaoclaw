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
            <el-icon><TrendCharts /></el-icon> 龙虾管理
          </el-menu-item>
          <el-menu-item index="skills">
            <el-icon><Ticket /></el-icon> Skill 库
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
                <el-table-column prop="type" label="类型" width="80">
                  <template #default="{ row }">
                    <el-tag size="small" :type="row.type === 'local' ? 'success' : 'info'" effect="dark">
                      {{ row.type === 'local' ? '本地' : 'SSH' }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="name" label="名称" min-width="100" />
                <el-table-column prop="region" label="区域" width="90">
                  <template #default="{ row }">{{ regionEmoji(row.region) }} {{ row.region }}</template>
                </el-table-column>
                <el-table-column prop="host" label="地址" min-width="140" font-mono />
                <el-table-column prop="remote_home" label="目录" min-width="180" show-overflow-tooltip />
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
                <el-table-column label="操作" width="240">
                  <template #default="{ row }">
                    <el-button size="small" @click="openEditNode(row)">编辑</el-button>
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

          <!-- ── Skill 库 ── -->
          <div v-show="activeTab === 'skills'">
            <el-card shadow="never" style="max-width: 860px; margin-bottom: 16px">
              <template #header><span><strong>上传 Skill zip 包</strong></span></template>
              <el-form label-width="140px">
                <el-form-item label="zip 包">
                  <input
                    ref="skillFileInput"
                    type="file"
                    accept=".zip"
                    style="display: none"
                    @change="onSkillFileSelected"
                  />
                  <div style="display: flex; gap: 12px; width: 100%; align-items: center">
                    <el-button @click="openSkillFilePicker">选择 zip</el-button>
                    <el-tag v-if="skillUploadFileName" type="info">{{ skillUploadFileName }}</el-tag>
                    <span v-else style="color: #909399">未选择文件</span>
                  </div>
                </el-form-item>
                <el-form-item label="Slug">
                  <el-input v-model="skillUploadForm.slug" placeholder="例如 firefly-iii-bookkeeper" />
                </el-form-item>
                <el-form-item label="显示名称">
                  <el-input v-model="skillUploadForm.display_name" placeholder="例如 Firefly III 记账" />
                </el-form-item>
                <el-form-item label="版本">
                  <el-input v-model="skillUploadForm.version" placeholder="例如 1.0.0" />
                </el-form-item>
                <el-form-item label="分类">
                  <el-input v-model="skillUploadForm.category" placeholder="例如 财务 / 数据 / 自动化" />
                </el-form-item>
                <el-form-item label="作者">
                  <el-input v-model="skillUploadForm.author" placeholder="例如 鲸奇互联" />
                </el-form-item>
                <el-form-item label="摘要">
                  <el-input v-model="skillUploadForm.summary" type="textarea" :rows="3" placeholder="这个 skill 是干什么的" />
                </el-form-item>
                <el-form-item label="标签">
                  <el-input v-model="skillUploadForm.tags" placeholder="逗号分隔，例如 记账,财务,Firefly" />
                </el-form-item>
                <el-form-item label="依赖二进制">
                  <el-input v-model="skillUploadForm.requires_bins" placeholder="逗号分隔，例如 python3,node" />
                </el-form-item>
                <el-form-item label="依赖环境变量">
                  <el-input v-model="skillUploadForm.requires_env" placeholder="逗号分隔，例如 FIREFLY_III_URL,FIREFLY_III_API_KEY" />
                </el-form-item>
                <el-form-item label="设为已验证">
                  <el-switch v-model="skillUploadForm.is_verified" />
                </el-form-item>
                <el-form-item>
                  <el-button type="primary" :loading="skillUploading" @click="uploadSkillPackage">
                    上传并入库
                  </el-button>
                </el-form-item>
              </el-form>
            </el-card>

            <el-card shadow="never">
              <template #header>
                <div style="display: flex; justify-content: space-between; align-items: center">
                  <span><strong>Skill 列表</strong></span>
                  <el-button size="small" @click="refreshSkillLibrary">刷新</el-button>
                </div>
              </template>
              <el-table :data="skillLibrary" stripe style="width: 100%">
                <el-table-column prop="display_name" label="名称" min-width="160" />
                <el-table-column prop="slug" label="Slug" min-width="160" />
                <el-table-column prop="version" label="版本" width="110" />
                <el-table-column prop="category" label="分类" width="120" />
                <el-table-column label="来源" width="120">
                  <template #default="{ row }">
                    <el-tag size="small" :type="row.source?.type === 'uploaded' ? 'success' : 'info'">
                      {{ row.source?.type || '-' }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="summary" label="摘要" min-width="220" show-overflow-tooltip />
                <el-table-column label="托管目录" min-width="220" show-overflow-tooltip>
                  <template #default="{ row }">{{ row.source?.local_dir || '-' }}</template>
                </el-table-column>
                <el-table-column label="操作" width="120">
                  <template #default="{ row }">
                    <el-popconfirm title="确认删除这个 Skill？" @confirm="deleteSkill(row.slug)">
                      <template #reference>
                        <el-button size="small" type="danger" plain>删除</el-button>
                      </template>
                    </el-popconfirm>
                  </template>
                </el-table-column>
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

            <el-card shadow="never" style="max-width: 760px; margin-top: 16px">
              <template #header><span><strong>Picoclaw 包管理</strong></span></template>
              <el-form label-width="180px">
                <el-form-item label="当前生效路径">
                  <el-input :model-value="packageInfo.resolved_path || '-'" disabled />
                </el-form-item>
                <el-form-item label="当前版本">
                  <el-input :model-value="packageInfo.version || '-'" disabled />
                </el-form-item>
                <el-form-item label="托管路径">
                  <el-input :model-value="packageInfo.managed_path || '-'" disabled />
                </el-form-item>
                <el-form-item label="手动替换包路径">
                  <el-input
                    v-model="packagePathInput"
                    placeholder="输入服务端本地文件路径，例如 /home/xxx/picoclaw/build/picoclaw"
                  />
                </el-form-item>
                <el-form-item>
                  <el-button type="primary" :loading="packageSaving" @click="applyPackagePath">
                    应用手动路径
                  </el-button>
                  <el-button type="success" :loading="packageFetching" @click="fetchLatestOfficialPackage">
                    获取官方最新并切换
                  </el-button>
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
        <el-form-item label="类型">
          <el-select v-model="newNode.type" style="width: 100%">
            <el-option label="SSH 节点" value="ssh" />
            <el-option label="本地节点" value="local" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="newNode.name" placeholder="如：华南-广州-01" />
        </el-form-item>
        <el-form-item :label="newNode.type === 'local' ? '节点标识' : 'IP 地址'">
          <el-input v-model="newNode.host" :placeholder="newNode.type === 'local' ? 'local 或 127.0.0.1' : '192.168.x.x 或 domain.com'" />
        </el-form-item>
        <el-form-item v-if="newNode.type === 'ssh'" label="SSH 用户">
          <el-input v-model="newNode.ssh_user" placeholder="默认 root" />
        </el-form-item>
        <el-form-item v-if="newNode.type === 'ssh'" label="SSH 端口">
          <el-input-number v-model="newNode.ssh_port" :min="1" :max="65535" style="width: 100%" />
        </el-form-item>
        <el-form-item v-if="newNode.type === 'ssh'" label="认证方式">
          <el-select v-model="newNode.ssh_auth_type" style="width: 100%">
            <el-option label="密码" value="password" />
            <el-option label="私钥路径" value="key_path" />
            <el-option label="私钥内容" value="private_key" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="newNode.type === 'ssh' && newNode.ssh_auth_type === 'password'" label="SSH 密码">
          <el-input v-model="newNode.ssh_password" type="password" show-password placeholder="root 密码" />
        </el-form-item>
        <el-form-item v-if="newNode.type === 'ssh' && newNode.ssh_auth_type === 'key_path'" label="私钥路径">
          <el-input v-model="newNode.ssh_key_path" placeholder="如：/root/.ssh/id_ed25519" />
        </el-form-item>
        <el-form-item v-if="newNode.type === 'ssh' && newNode.ssh_auth_type === 'private_key'" label="私钥内容">
          <el-input
            v-model="newNode.ssh_private_key"
            type="textarea"
            :rows="6"
            placeholder="粘贴 OPENSSH PRIVATE KEY 内容"
          />
        </el-form-item>
        <el-form-item v-if="newNode.type === 'ssh' && newNode.ssh_auth_type !== 'password'" label="证书路径">
          <el-input v-model="newNode.ssh_certificate_path" placeholder="可选，如：/root/.ssh/id_ed25519-cert.pub" />
        </el-form-item>
        <el-form-item v-if="newNode.type === 'local'" label="本地目录">
          <el-input v-model="newNode.remote_home" placeholder="如：/home/you/.pinhaoclaw/local-node-01" />
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

    <!-- 编辑节点对话框 -->
    <el-dialog v-model="editNodeVisible" title="编辑节点" width="480px" destroy-on-close>
      <el-form :model="editNode" label-width="80px">
        <el-form-item label="类型">
          <el-select v-model="editNode.type" style="width: 100%">
            <el-option label="SSH 节点" value="ssh" />
            <el-option label="本地节点" value="local" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="editNode.name" placeholder="如：华南-广州-01" />
        </el-form-item>
        <el-form-item :label="editNode.type === 'local' ? '节点标识' : 'IP 地址'">
          <el-input v-model="editNode.host" :placeholder="editNode.type === 'local' ? 'local 或 127.0.0.1' : '192.168.x.x 或 domain.com'" />
        </el-form-item>
        <el-form-item v-if="editNode.type === 'ssh'" label="SSH 用户">
          <el-input v-model="editNode.ssh_user" placeholder="默认 root" />
        </el-form-item>
        <el-form-item v-if="editNode.type === 'ssh'" label="SSH 端口">
          <el-input-number v-model="editNode.ssh_port" :min="1" :max="65535" style="width: 100%" />
        </el-form-item>
        <el-form-item v-if="editNode.type === 'ssh'" label="认证方式">
          <el-select v-model="editNode.ssh_auth_type" style="width: 100%">
            <el-option label="保持现有" value="" />
            <el-option label="密码（更新）" value="password" />
            <el-option label="私钥路径（更新）" value="key_path" />
            <el-option label="私钥内容（更新）" value="private_key" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="editNode.type === 'ssh' && editNode.ssh_auth_type === 'password'" label="SSH 密码">
          <el-input v-model="editNode.ssh_password" type="password" show-password placeholder="不填则保持原密码" />
        </el-form-item>
        <el-form-item v-if="editNode.type === 'ssh' && editNode.ssh_auth_type === 'key_path'" label="私钥路径">
          <el-input v-model="editNode.ssh_key_path" placeholder="如：/root/.ssh/id_ed25519" />
        </el-form-item>
        <el-form-item v-if="editNode.type === 'ssh' && editNode.ssh_auth_type === 'private_key'" label="私钥内容">
          <el-input
            v-model="editNode.ssh_private_key"
            type="textarea"
            :rows="6"
            placeholder="粘贴 OPENSSH PRIVATE KEY 内容"
          />
        </el-form-item>
        <el-form-item v-if="editNode.type === 'ssh' && (editNode.ssh_auth_type === 'key_path' || editNode.ssh_auth_type === 'private_key')" label="证书路径">
          <el-input v-model="editNode.ssh_certificate_path" placeholder="可选，如：/root/.ssh/id_ed25519-cert.pub" />
        </el-form-item>
        <el-form-item v-if="editNode.type === 'local'" label="本地目录">
          <el-input v-model="editNode.remote_home" placeholder="如：/home/you/.pinhaoclaw/local-node-01" />
        </el-form-item>
        <el-form-item label="区域">
          <el-select v-model="editNode.region" style="width: 100%">
            <el-option label="☀️ 华南" value="华南" />
            <el-option label="❄️ 华北" value="华北" />
            <el-option label="🌤️ 华中" value="华中" />
            <el-option label="🌊 华东" value="华东" />
            <el-option label="🌐 境外" value="境外" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editNodeVisible = false">取消</el-button>
        <el-button type="primary" :loading="nodeEditing" @click="doUpdateNode">保存修改</el-button>
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
  type PicoclawPackageInfo,
  type SkillRegistryEntry,
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
  TrendCharts,
} from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
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
  { key: "skills", label: "Skill 库" },
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
const skillLibrary = ref<SkillRegistryEntry[]>([]);
const settings = ref<Settings>({
  default_monthly_token_limit: 1000000,
  default_monthly_space_limit_mb: 2048,
  default_max_lobsters_per_user: 3,
});
const settingsForm = ref({ tokenLimitW: 100, spaceLimitMB: 2048, maxLobsters: 3 });
const packageInfo = ref<PicoclawPackageInfo>({
  configured_path: "",
  resolved_path: "",
  version: "",
  managed_path: "",
});
const packagePathInput = ref("");
const packageSaving = ref(false);
const packageFetching = ref(false);
const skillUploading = ref(false);
const skillFileInput = ref<HTMLInputElement | null>(null);
const skillUploadFile = ref<File | null>(null);
const skillUploadFileName = ref("");
const skillUploadForm = ref({
  slug: "",
  display_name: "",
  summary: "",
  category: "",
  author: "",
  version: "",
  icon: "",
  tags: "",
  requires_bins: "",
  requires_env: "",
  is_verified: true,
});

// 添加节点
const addNodeVisible = ref(false);
const nodeAdding = ref(false);
const newNode = ref({
  type: "ssh",
  name: "新节点",
  host: "",
  ssh_port: 22,
  ssh_user: "root",
  ssh_auth_type: "password",
  ssh_password: "",
  ssh_key_path: "",
  ssh_private_key: "",
  ssh_certificate_path: "",
  ssh_key_passphrase: "",
  remote_home: "",
  region: "华南",
});

const editNodeVisible = ref(false);
const nodeEditing = ref(false);
const editNode = ref<any>({
  id: "",
  type: "ssh",
  name: "",
  host: "",
  ssh_port: 22,
  ssh_user: "root",
  ssh_auth_type: "",
  ssh_password: "",
  ssh_key_path: "",
  ssh_private_key: "",
  ssh_certificate_path: "",
  ssh_key_passphrase: "",
  remote_home: "",
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
    try {
      await loadAll();
    } catch {
      // Token expired or invalid - show login dialog
      isAuthed.value = false;
      loginVisible.value = true;
      uni.removeStorageSync("pc_admin_token");
    }
  }
});

/** 验证当前 URL 路径是否匹配后端配置的 AdminPath */
async function verifyGate() {
  // #ifdef H5
  // 优先读取 pathname（例如 /mgr-x7Kp9qZ），避免被 hash 路由误判
  const pathname = window.location.pathname || '';
  const pathnameSegment = pathname.split('/').filter(Boolean)[0] || '';

  // 兼容旧逻辑：当 pathname 无法提供路径段时，回退到 hash
  // 例：/#/pages/admin/index
  const hashPath = window.location.hash.replace(/^#/, '').split('?')[0];
  const hashSegment = hashPath.split('/').filter(Boolean)[0] || '';
  const firstSegment = pathnameSegment || hashSegment;

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
  try {
    const res = await adminApi.login(adminPwd.value);
    if (res.ok) {
      uni.setStorageSync("pc_admin_token", res.token);
      isAuthed.value = true;
      loginVisible.value = false;
      loadAll();
    } else {
      loginErr.value = res.message || "密码错误";
    }
  } catch {
    loginErr.value = "网络错误，请重试";
  }
}

async function loadAll() {
  await Promise.all([
    adminApi.overview().then((d) => (overview.value = d)).catch(() => {}),
    adminApi.nodes().then((d) => (nodes.value = d)).catch(() => {}),
    adminApi.invites().then((d) => (invites.value = d)).catch(() => {}),
    adminApi.lobsters().then((d) => (allLobsters.value = d)).catch(() => {}),
    adminApi.skills().then((d) => (skillLibrary.value = d.skills || [])).catch(() => {}),
    adminApi.settings().then((d) => {
      settings.value = d;
      settingsForm.value = {
        tokenLimitW: Math.round(d.default_monthly_token_limit / 10000),
        spaceLimitMB: d.default_monthly_space_limit_mb,
        maxLobsters: d.default_max_lobsters_per_user,
      };
    }).catch(() => {}),
    adminApi.picoclawPackage().then((d) => {
      packageInfo.value = d;
      packagePathInput.value = d.configured_path || d.resolved_path || "";
    }).catch(() => {}),
  ]);
}

function openSkillFilePicker() {
  skillFileInput.value?.click();
}

function onSkillFileSelected(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0] || null;
  skillUploadFile.value = file;
  skillUploadFileName.value = file?.name || "";
  if (!skillUploadForm.value.slug && file?.name) {
    skillUploadForm.value.slug = file.name
      .replace(/\.zip$/i, "")
      .trim()
      .toLowerCase()
      .replace(/\s+/g, "-");
  }
  if (!skillUploadForm.value.display_name && file?.name) {
    skillUploadForm.value.display_name = file.name.replace(/\.zip$/i, "").trim();
  }
}

async function refreshSkillLibrary() {
  try {
    const res = await adminApi.skills();
    skillLibrary.value = res.skills || [];
  } catch (err: any) {
    ElMessage.error(err?.message || "加载 Skill 列表失败");
  }
}

async function uploadSkillPackage() {
  if (!skillUploadFile.value) {
    ElMessage.warning("请先选择 zip 包");
    return;
  }
  skillUploading.value = true;
  try {
    await adminApi.uploadSkillZip(skillUploadFile.value, skillUploadForm.value);
    ElMessage.success("Skill 已上传并入库");
    skillUploadFile.value = null;
    skillUploadFileName.value = "";
    if (skillFileInput.value) {
      skillFileInput.value.value = "";
    }
    skillUploadForm.value = {
      slug: "",
      display_name: "",
      summary: "",
      category: "",
      author: "",
      version: "",
      icon: "",
      tags: "",
      requires_bins: "",
      requires_env: "",
      is_verified: true,
    };
    await refreshSkillLibrary();
  } catch (err: any) {
    ElMessage.error(err?.message || "上传 Skill 失败");
  } finally {
    skillUploading.value = false;
  }
}

async function deleteSkill(slug: string) {
  try {
    await adminApi.deleteSkill(slug);
    ElMessage.success("Skill 已删除");
    await refreshSkillLibrary();
  } catch (err: any) {
    ElMessage.error(err?.message || "删除 Skill 失败");
  }
}

async function doAddNode() {
  if (newNode.value.type === "local") {
    if (!newNode.value.remote_home) {
      ElMessage.warning("请输入本地目录");
      return;
    }
    if (!newNode.value.host) {
      newNode.value.host = "local";
    }
  } else if (!newNode.value.host) {
    ElMessage.warning("请输入节点地址");
    return;
  } else {
    if (!newNode.value.ssh_user) {
      newNode.value.ssh_user = "root";
    }
    if (!newNode.value.ssh_port || newNode.value.ssh_port <= 0) {
      newNode.value.ssh_port = 22;
    }

    if (newNode.value.ssh_auth_type === "password" && !newNode.value.ssh_password) {
      ElMessage.warning("请输入 SSH 密码");
      return;
    }
    if (newNode.value.ssh_auth_type === "key_path" && !newNode.value.ssh_key_path) {
      ElMessage.warning("请输入私钥路径");
      return;
    }
    if (newNode.value.ssh_auth_type === "private_key" && !newNode.value.ssh_private_key) {
      ElMessage.warning("请输入私钥内容");
      return;
    }

    if (newNode.value.ssh_auth_type !== "password") {
      newNode.value.ssh_password = "";
    }
    if (newNode.value.ssh_auth_type !== "key_path") {
      newNode.value.ssh_key_path = "";
    }
    if (newNode.value.ssh_auth_type !== "private_key") {
      newNode.value.ssh_private_key = "";
    }
  }
  nodeAdding.value = true;
  await adminApi
    .addNode(newNode.value)
    .then(() => {
      addNodeVisible.value = false;
      newNode.value = {
        type: "ssh",
        name: "新节点",
        host: "",
        ssh_port: 22,
        ssh_user: "root",
        ssh_auth_type: "password",
        ssh_password: "",
        ssh_key_path: "",
        ssh_private_key: "",
        ssh_certificate_path: "",
        ssh_key_passphrase: "",
        remote_home: "",
        region: "华南",
      };
      adminApi.nodes().then((d) => (nodes.value = d));
    })
    .catch(() => {});
  nodeAdding.value = false;
}

function openEditNode(node: Node) {
  editNode.value = {
    id: node.id,
    type: node.type || "ssh",
    name: node.name || "",
    host: node.host || "",
    ssh_port: node.ssh_port || 22,
    ssh_user: node.ssh_user || "root",
    ssh_auth_type: "",
    ssh_password: "",
    ssh_key_path: "",
    ssh_private_key: "",
    ssh_certificate_path: node.ssh_certificate_path || "",
    ssh_key_passphrase: "",
    remote_home: node.remote_home || "",
    region: node.region || "华南",
  };
  editNodeVisible.value = true;
}

async function doUpdateNode() {
  if (!editNode.value.id) {
    ElMessage.warning("缺少节点 ID");
    return;
  }

  if (editNode.value.type === "local") {
    if (!editNode.value.remote_home) {
      ElMessage.warning("请输入本地目录");
      return;
    }
    if (!editNode.value.host) {
      editNode.value.host = "local";
    }
  } else {
    if (!editNode.value.host) {
      ElMessage.warning("请输入节点地址");
      return;
    }
    if (!editNode.value.ssh_user) {
      editNode.value.ssh_user = "root";
    }
    if (!editNode.value.ssh_port || editNode.value.ssh_port <= 0) {
      editNode.value.ssh_port = 22;
    }

    if (editNode.value.ssh_auth_type === "password" && !editNode.value.ssh_password) {
      ElMessage.warning("请输入 SSH 密码");
      return;
    }
    if (editNode.value.ssh_auth_type === "key_path" && !editNode.value.ssh_key_path) {
      ElMessage.warning("请输入私钥路径");
      return;
    }
    if (editNode.value.ssh_auth_type === "private_key" && !editNode.value.ssh_private_key) {
      ElMessage.warning("请输入私钥内容");
      return;
    }
  }

  const payload: any = {
    type: editNode.value.type,
    name: editNode.value.name,
    host: editNode.value.host,
    ssh_port: editNode.value.ssh_port,
    ssh_user: editNode.value.ssh_user,
    remote_home: editNode.value.remote_home,
    region: editNode.value.region,
  };

  if (editNode.value.type === "ssh") {
    if (editNode.value.ssh_auth_type === "password") {
      payload.ssh_password = editNode.value.ssh_password;
    }
    if (editNode.value.ssh_auth_type === "key_path") {
      payload.ssh_key_path = editNode.value.ssh_key_path;
      payload.ssh_certificate_path = editNode.value.ssh_certificate_path;
    }
    if (editNode.value.ssh_auth_type === "private_key") {
      payload.ssh_private_key = editNode.value.ssh_private_key;
      payload.ssh_certificate_path = editNode.value.ssh_certificate_path;
      payload.ssh_key_passphrase = editNode.value.ssh_key_passphrase;
    }
  }

  nodeEditing.value = true;
  await adminApi
    .updateNode(editNode.value.id, payload)
    .then(() => {
      editNodeVisible.value = false;
      ElMessage.success("节点已更新");
      return adminApi.nodes().then((d) => (nodes.value = d));
    })
    .catch((err: any) => {
      ElMessage.error(err?.message || "更新节点失败");
    });
  nodeEditing.value = false;
}

async function testNode(id: string) {
  try {
    const res = await adminApi.testNode(id);
    // #ifdef H5
    if (res.ok) {
      ElMessage.success("连接成功");
    } else {
      ElMessage.warning(res.message || "连接失败");
    }
    // #endif
    adminApi.nodes().then((d) => (nodes.value = d));
  } catch {
    // #ifdef H5
    ElMessage.error("测试连接失败");
    // #endif
  }
}

async function deleteNode(id: string) {
  try {
    await adminApi.deleteNode(id);
    const d = await adminApi.nodes();
    nodes.value = d;
  } catch {
    // #ifdef H5
    ElMessage.error("删除节点失败");
    // #endif
  }
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
  navigator.clipboard?.writeText(url).then(() => {
    ElMessage.success("链接已复制");
  }).catch(() => {
    ElMessage.error("复制失败");
  });
  // #endif
}

async function deleteInvite(code: string) {
  try {
    await adminApi.deleteInvite(code);
    const d = await adminApi.invites();
    invites.value = d;
  } catch {
    // #ifdef H5
    ElMessage.error("删除邀请码失败");
    // #endif
  }
}

async function saveSettings() {
  try {
    await adminApi.updateSettings({
      default_monthly_token_limit:
        (Number(settingsForm.value.tokenLimitW) || 100) * 10000,
      default_monthly_space_limit_mb:
        Number(settingsForm.value.spaceLimitMB) || 2048,
      default_max_lobsters_per_user:
        Number(settingsForm.value.maxLobsters) || 3,
    });
    // #ifdef H5
    ElMessage.success("设置已保存");
    // #endif
  } catch {
    // #ifdef H5
    ElMessage.error("保存失败");
    // #endif
  }
}

async function applyPackagePath() {
  if (!packagePathInput.value.trim()) {
    ElMessage.warning("请输入包路径");
    return;
  }
  packageSaving.value = true;
  try {
    const res = await adminApi.setPicoclawPackage(packagePathInput.value.trim());
    packageInfo.value = res.package;
    packagePathInput.value = res.package.configured_path || res.package.resolved_path || "";
    ElMessage.success("已切换为手动包路径");
  } catch (err: any) {
    ElMessage.error(err?.message || "手动切换失败");
  } finally {
    packageSaving.value = false;
  }
}

async function fetchLatestOfficialPackage() {
  packageFetching.value = true;
  try {
    const res = await adminApi.fetchLatestPicoclawPackage();
    packageInfo.value = res.package;
    packagePathInput.value = res.package.configured_path || res.package.resolved_path || "";
    ElMessage.success("官方最新包已下载并切换");
  } catch (err: any) {
    ElMessage.error(err?.message || "获取官方包失败");
  } finally {
    packageFetching.value = false;
  }
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

.sidebar-footer {
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
