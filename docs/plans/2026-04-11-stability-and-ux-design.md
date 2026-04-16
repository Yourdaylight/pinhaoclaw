# PinHaoClaw 稳定基石与体验增强设计

> 日期: 2026-04-11
> 定位: SaaS 商业化，先 C 端后 B 端
> 核心卖点: 极简上手体验
> 策略: 先修 bug 保稳定，再做体验增强

---

## P0: Critical Bug 修复

### 后端 P0（安全相关）

#### 1. JWT 认证绕过修复
- **位置**: `server/casdoor.go:181-186`
- **问题**: `ParseIdentity` 验签失败时回退到不验签解析 `decodeJWTClaims`，攻击者可伪造任意 JWT
- **方案**: 移除不验签回退，验签失败直接返回错误
- **代码变更**:
  ```go
  // 修改前
  claims, err := c.verifyAndParseJWT(context.Background(), rawToken)
  if err != nil {
      claims = &casdoorClaims{}
      if err2 := decodeJWTClaims(rawToken, claims); err2 != nil {
          return nil, fmt.Errorf("JWT 验签失败且无法解析: %w", err)
      }
  }

  // 修改后
  claims, err := c.verifyAndParseJWT(context.Background(), rawToken)
  if err != nil {
      return nil, fmt.Errorf("JWT 验签失败: %w", err)
  }
  ```

#### 2. SSH 命令注入修复
- **位置**: `claw/service.go` 多处（105, 226, 232-236, 244-246, 254-259, 267, 281-287, 307, 309）
- **问题**: 用户可控值直接 `fmt.Sprintf` 拼入 shell 命令
- **方案**: 加输入校验 + shellescape
  - lobsterID/instDir: 白名单校验 `^[a-zA-Z0-9_-]+$`
  - remoteHome: 校验路径格式 + shellescape
  - port: 校验数字范围 8100-8300
- **依赖**: 引入 `kballard/go-shellquote` 或手写 `shellescape` 函数

#### 3. Token Query 参数限制
- **位置**: `server/app.go:175`
- **问题**: 所有路由都接受 `?token=` 参数，token 泄露到日志/浏览器历史
- **方案**: 仅 WebSocket 路由允许 query token，其他路由只从 Header 取
  ```go
  // requireUser 中间件
  token := c.GetHeader("Authorization")
  if token == "" {
      token = c.GetHeader("X-User-Token")
  }
  // 只在 WebSocket 路由接受 query token
  if token == "" && c.Request.URL.Path == "/api/lobsters/bind-ws" {
      token = c.Query("token")
  }
  ```

#### 4. 竞态条件修复
- **位置**: `server/app.go` 多处（307-365, 220-248, 363-364, 549, 182-191）
- **问题**: check-and-act 操作无锁保护（龙虾创建超配额、邀请码超限、节点计数不一致）
- **方案**: 添加 `sync.Mutex` 保护关键操作
  - `handleCreateLobster`: 锁内检查配额 + 创建
  - `handleUserLogin`: 锁内检查邀请码 + 创建用户
  - `handleDeleteLobster`: 锁内更新节点计数
  - `requireUser`: 读操作使用 RWMutex 的 RLock

#### 5. SSH 密码加密存储
- **位置**: `sharing/store.go:267`
- **问题**: nodes.json 中 SSH 密码明文存储
- **方案**: AES-GCM 加密存储，密钥从环境变量 `PINHAOCLAW_ENCRYPT_KEY` 读取
  - 存储: `SaveNode` 时加密 `ssh_password`
  - 读取: `ReadNode` 时解密
  - 兼容: 无加密前缀的视为明文，首次读取后重写为密文

### 前端 P0（体验阻断）

#### 6. 属性名大小写修复
- **位置**: `panel/index.vue:73,79`
- **问题**: H5 模板使用 `l.weixinBound`/`l.createdAt`，但接口返回 `weixin_bound`/`created_at`
- **方案**: 统一为 snake_case（与后端一致）
  ```vue
  <el-tag :type="l.weixin_bound ? 'success' : 'info'">
  <span>{{ formatTime(l.created_at) }}</span>
  ```

#### 7. Coral 图标替换
- **位置**: `admin/index.vue:57`
- **问题**: Element Plus 无 `Coral` 图标
- **方案**: 替换为 `ChromeFilled` 或自定义龙虾 SVG 图标

#### 8. loading 卡死修复
- **位置**: `stores/lobster.ts:9-13`
- **问题**: API 报错时 `loading.value` 永远不重置
- **方案**: 所有 store 方法加 `try/finally`
  ```ts
  async function fetchAll() {
    loading.value = true;
    try {
      lobsters.value = await lobsterApi.list();
    } finally {
      loading.value = false;
    }
  }
  ```

#### 9. clearStorageSync 修复
- **位置**: `stores/user.ts:42`
- **问题**: `uni.clearStorageSync()` 清除所有存储，包括管理员 token
- **方案**: 只清除 `pc_user_*` 前缀的 key
  ```ts
  function logout() {
    const keysToKeep = Object.keys(uni.getStorageInfoSync()).filter(k => !k.startsWith('pc_user_'));
    uni.clearStorageSync();
    keysToKeep.forEach(k => { /* restore */ });
    // 或者: 只删除特定 key
    ['pc_user_token', 'pc_user_id', 'pc_user_name', 'pc_max_lobsters'].forEach(k => uni.removeStorageSync(k));
  }
  ```

#### 10. 操作反馈补全
- **位置**: `panel/index.vue:328-344`，`admin/index.vue` 多处
- **问题**: 停止/启动/删除/保存等操作无成功/失败反馈
- **方案**: 所有操作加 uni.showToast / ElMessage 反馈
  ```ts
  async function onStop(id: string) {
    try {
      await lobsterStore.stop(id);
      uni.showToast({ title: '已停止', icon: 'success' });
    } catch (e) {
      uni.showToast({ title: '操作失败', icon: 'error' });
    }
  }
  ```

#### 11. 小程序占位符替换
- **位置**: `request.ts:8`，`manifest.json:9`
- **问题**: BASE_URL 为 `https://your-server.com`，appid 为空
- **方案**: 替换为真实服务器地址和微信 appid

#### 12. JSON.parse 容错
- **位置**: `bind.vue` 多处（143, 147, 150, 157, 180）
- **问题**: SSE/WebSocket 消息解析无 try/catch
- **方案**: 封装 `safeJSONParse` 函数
  ```ts
  function safeJSONParse<T>(data: string, fallback: T): T {
    try { return JSON.parse(data); }
    catch { return fallback; }
  }
  ```

#### 13. 其他 High 级修复
- **401 handler 区分用户/管理员 token**: `request.ts:56-60` 检查是否为 admin 请求，清除对应 token
- **Admin token 过期处理**: `admin/index.vue:371-376` 过期时显示登录框而非跳转用户登录页
- **Casdoor 回调自动提交**: `login/index.vue:166-175` 修正条件判断，回调时自动提交 code
- **CORS 限制**: 后端 `app.go:873` 将 `*` 改为只允许前端域名
- **Admin 路径枚举**: `app.go:578-595` gate 端点加速率限制

---

## P1: 极简体验增强

### 1. 新用户引导流

**问题**: 用户注册后看到空面板，不知道下一步。

**方案**: 首次登录检测 → 自动弹创建向导 → 3 步完成

**流程**:
1. 用户首次登录 → `GET /api/users/me` 返回 `is_first_login: true`
2. 前端自动弹出向导对话框（非独立页面，减少跳转）
3. Step 1: 选区域（默认推荐延迟最低的节点，用 `GET /api/nodes/regions` 获取）
4. Step 2: 确认创建（一键，无需额外配置）
5. Step 3: 扫码绑微信（自动弹出二维码，复用现有绑定流程）

**后端改动**:
- `User` 结构体新增 `FirstLoginAt *time.Time`
- `GET /api/users/me` 返回 `is_first_login`（`FirstLoginAt` 为 nil 则为 true）
- 首次 me 请求后自动设置 `FirstLoginAt`

**前端改动**:
- `panel/index.vue` 的 `onMounted` 检测 `is_first_login`，自动打开 `OnboardingDialog`
- 新增 `components/OnboardingDialog.vue`：3 步向导组件
- 完成后 `reLaunch` 刷新面板

### 2. 龙虾状态实时更新

**问题**: 用户必须手动刷新才能看到龙虾状态变化。

**方案**: SSE 推送龙虾状态变更事件

**后端改动**:
- 新增 `GET /api/lobsters/events` SSE 端点
- 龙虾状态变更时（创建/绑定/启动/停止/删除），向订阅者推送事件
  ```json
  {"type": "lobster_status", "id": "xxx", "status": "running", "updated_at": "..."}
  ```
- 使用 channel 广播机制：每个 SSE 连接注册一个 channel，状态变更时遍历发送
- 心跳：每 30 秒发送 `{"type": "ping"}`
- 认证：通过 Header `X-User-Token` 传递（不在 URL 中）

**前端改动**:
- panel 页 `onMounted` 建立 SSE 连接，`onUnmounted` 关闭
- 收到事件后更新对应龙虾卡片状态
- 断线自动重连（3 秒延迟，指数退避）
- 小程序端：使用 `uni.connectSocket` + WebSocket 替代 SSE

### 3. 一键续费/升级

**问题**: 配额用完后无提醒，用户不知道怎么办。

**方案**: 配额预警 + 升级入口

**后端改动**:
- 龙虾配额达到 80% 时，在 SSE 事件中推送 `{"type": "quota_warning", "id": "xxx", "usage_percent": 85}`
- 新增 `POST /api/billing/checkout`：创建 Casdoor 支付会话，返回支付 URL
- 新增 `POST /api/billing/callback`：Casdoor 支付回调 Webhook，更新配额
- 与 Casdoor Product/Plan 体系对接（参见 MEMORY.md 中的产品清单）

**前端改动**:
- `QuotaBar` 组件：使用量超过 80% 变黄色，100% 变红色
- 点击 QuotaBar 或 "升级" 按钮弹出升级选项
- 升级选项：按量续费（补充配额）或 包月升级（更换 Plan）
- 支付成功后自动刷新配额数据

### 4. 移动端体验打磨

**小程序端优化**:
- `LobsterCard.vue` 适配小程序样式（当前偏向 H5 设计）
- 危险操作（删除、停止）加 `uni.showModal` 确认对话框
- 龙虾列表支持下拉刷新（`onPullDownRefresh`）
- 加载状态使用骨架屏替代空白

**H5 端优化**:
- 响应式布局：窄屏时卡片单列
- 龙虾详情页（当前只有列表，无详情页）
- 操作日志查看（绑定日志回看）

---

## 实施顺序

| 序号 | 任务 | 阶段 | 依赖 |
|------|------|------|------|
| 1 | JWT 认证绕过修复 | P0 | 无 |
| 2 | SSH 命令注入修复 | P0 | 无 |
| 3 | Token Query 参数限制 | P0 | 无 |
| 4 | 竞态条件修复 | P0 | 无 |
| 5 | SSH 密码加密存储 | P0 | 无 |
| 6 | 属性名大小写修复 | P0 | 无 |
| 7 | Coral 图标替换 | P0 | 无 |
| 8 | loading 卡死修复 | P0 | 无 |
| 9 | clearStorageSync 修复 | P0 | 无 |
| 10 | 操作反馈补全 | P0 | 无 |
| 11 | 小程序占位符替换 | P0 | 需要真实服务器地址和 appid |
| 12 | JSON.parse 容错 | P0 | 无 |
| 13 | 其他 High 级修复 | P0 | 无 |
| 14 | 新用户引导流 | P1 | P0 完成 |
| 15 | 龙虾状态实时更新 | P1 | P0 完成 |
| 16 | 一键续费/升级 | P1 | P0 完成 + Casdoor 产品配置 |
| 17 | 移动端体验打磨 | P1 | P0 完成 |
