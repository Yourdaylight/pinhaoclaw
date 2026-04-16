# PinHaoClaw Sidecar 认证集成指南

PinHaoClaw 通过 `casdoor-auth-sidecar` 接入 Casdoor 统一认证，实现 OAuth2 授权码登录。本文档记录完整的集成步骤、配置项和流程细节。

## 架构概览

```
浏览器
  │
  ├─ GET /api/auth/sidecar/login ──► pinhaoclaw (proxy) ──► sidecar ──► 302 to Casdoor
  │
  ├─ 用户在 Casdoor 登录页输入凭据
  │
  ├─ Casdoor 302 回调 ──► GET /api/auth/sidecar/callback ──► pinhaoclaw (proxy) ──► sidecar
  │                                                                                    │
  │                                                                          exchange code → JWT
  │                                                                          create session
  │                                                                          return bridge HTML
  │
  ├─ bridge HTML 写入 localStorage: casdoor_auth_token
  │
  └─ 后续 API 请求带 X-User-Token ──► pinhaoclaw ──► POST sidecar /api/auth/verify ──► 返回用户身份
```

**关键设计**：浏览器永远不直接访问 sidecar（sidecar 只监听 localhost:9098），所有请求通过 pinhaoclaw 代理。

## 前置条件

### 1. Casdoor 服务

- 部署地址（示例）：`https://auth.example.com`
- 创建应用（如 `app_pinhaoclaw_jq`），获取 `client_id` 和 `client_secret`
- 应用设置：
  - `type`: All
  - `grantTypes`: `["authorization_code","password","client_credentials","refresh_token"]`
  - `enablePassword`: true
  - `redirect_uris`: 包含 `http://<你的IP>:<端口>/api/auth/sidecar/callback`

### 2. casdoor-auth-sidecar 二进制

确保 `casdoor-auth-sidecar` 二进制已安装并在部署环境中可执行。

## 配置步骤

### Step 1：配置 sidecar

编辑 `~/.casdoor-auth-sidecar/config.yaml`：

```yaml
casdoor:
   endpoint: https://auth.example.com          # Casdoor 地址
   client_id: your-client-id                   # 应用 client_id
   client_secret: your-client-secret           # 应用 client_secret
   organization: your-org                      # Casdoor 组织名
   application: your-app                       # 应用名
    redirect_path: /api/auth/sidecar/callback   # 回调路径（注意：经 pinhaoclaw 代理）
server:
    listen: :9098                               # sidecar 监听端口（仅 localhost）
   public_origin: https://pinhaoclaw.example.com    # pinhaoclaw 对外地址（决定回调 URL）
```

**关键点**：
- `redirect_path` 必须是 `/api/auth/sidecar/callback`（pinhaoclaw 的代理路由）
- `public_origin` 必须是用户浏览器能访问到的 pinhaoclaw 地址
- 生成的 `redirect_uri` = `public_origin` + `redirect_path`

### Step 2：配置 PinHaoClaw

编辑 `pinhaoclaw/.env`：

```env
PINHAOCLAW_AUTH_MODE=sidecar
PINHAOCLAW_AUTH_SIDECAR_URL=http://localhost:9098
PINHAOCLAW_PUBLIC_ORIGIN=https://pinhaoclaw.example.com
PINHAOCLAW_CASDOOR_ENDPOINT=https://auth.example.com
```

- `AUTH_MODE=sidecar` 启用 sidecar 认证模式
- `AUTH_SIDECAR_URL` 是 sidecar 的内部地址（后端直接调用）
- `PUBLIC_ORIGIN` 必须与 sidecar 的 `public_origin` 一致
- `CASDOOR_ENDPOINT` 是 Casdoor 的外部访问地址（用于生成前端配置信息）

### Step 3：Casdoor 应用配置

确保 Casdoor 应用的 `redirect_uris` 包含回调地址：

```
https://pinhaoclaw.example.com/api/auth/sidecar/callback
```

如果要同时支持 localhost 开发环境，也加上：

```
http://localhost:9000/api/auth/sidecar/callback
```

确保应用的 `grantTypes` 不为空（必须包含 `authorization_code`）。

### Step 4：启动服务

```bash
# 1. 启动 sidecar
cd ~/jqyl/casdoor-auth-sidecar
nohup ./casdoor-auth-sidecar > /tmp/sidecar.log 2>&1 &

# 2. 启动 pinhaoclaw
cd ~/jqyl/pinhaoclaw
./pinhaoclaw -p 9000
```

## 涉及的代码文件

### 后端 (Go)

| 文件 | 作用 |
|------|------|
| `config/config.go` | `SidecarEnabled()` 判断、`AuthSidecarURL`/`PublicOrigin`/`CasdoorEndpoint` 配置 |
| `server/sidecar.go` | `SidecarClient` (Verify)、代理 handlers (login/callback/logout)、`findOrCreateSidecarUser`、`handleAuthConfig` |
| `server/app.go` | 路由注册、`requireUser()` 中间件调用 sidecar.Verify |

### Sidecar (Go)

| 文件 | 作用 |
|------|------|
| `handlers.go` | `/api/auth/login`（支持 `?prompt=` 参数）、`/api/auth/callback`、`/api/auth/logout`、`/api/auth/verify` |
| `client.go` | `LoginURLWithPrompt(prompt)` — 生成带 `prompt=login` 的 Casdoor 授权 URL |
| `config.go` | sidecar 自身配置（Casdoor endpoint、client_id 等） |

### 前端 (Vue3/uni-app)

| 文件 | 作用 |
|------|------|
| `src/api/auth.ts` | `authApi.config()`/`.logout()` 定义 |
| `src/api/request.ts` | 请求拦截，优先读 `casdoor_auth_token`，401 自动清 token |
| `src/stores/user.ts` | `logout()` — 清本地 token + 设 `force_relogin` 标记 + 跳转后端 logout |
| `src/pages/login/index.vue` | `goSidecarLogin()` — 检测 `force_relogin`，自动追加 `prompt=login`；`onMounted` — 检测回调 token |
| `src/pages/panel/index.vue` | 顶栏显示 `userStore.userName`，退出按钮调 `userStore.logout()` |

## 后端路由

```
GET  /api/auth/config              → 返回 {mode, sidecar_enabled, login_url, logout_url}
GET  /api/auth/sidecar/login       → 代理 sidecar → 302 到 Casdoor（支持 ?prompt=login）
GET  /api/auth/sidecar/callback    → 代理 sidecar → 返回 bridge HTML（写 token）
GET  /api/auth/sidecar/logout      → 销毁 sidecar session → 302 回应用首页
GET  /api/auth/me                  → requireUser() → 返回用户信息
```

## 认证流程详解

### 登录流程

```
1. 用户打开 https://pinhaoclaw.example.com/
2. 前端加载 login/index.vue
3. onMounted 检查:
   - 如果 localStorage 有 casdoor_auth_token → fetchMe() → 跳转面板
   - 否则显示登录页（"前往统一认证中心"按钮）

4. 用户点击"前往统一认证中心"
5. goSidecarLogin():
   a. 检查 localStorage 是否有 force_relogin 标记
   b. 如果有 → 在 login_url 后追加 ?prompt=login，删除标记
   c. window.location.href = login_url（含或不含 prompt=login）

6. 浏览器请求 GET /api/auth/sidecar/login[?prompt=login]
7. pinhaoclaw 代理到 sidecar GET /api/auth/login[?prompt=login]
8. sidecar 生成 OAuth2 state，返回 302 到 Casdoor 授权页
   - URL 包含 prompt=login 时，Casdoor 强制显示登录表单（不自动授权）

9. 用户在 Casdoor 输入账号密码
10. Casdoor 生成 auth code，302 回调到:
   https://pinhaoclaw.example.com/api/auth/sidecar/callback?code=xxx&state=xxx

11. pinhaoclaw 代理到 sidecar GET /api/auth/callback
12. sidecar 用 code 换取 JWT + refresh_token，创建本地 session，返回 bridge HTML

13. bridge HTML 在浏览器执行:
    a. localStorage.setItem("casdoor_auth_token", sessionToken)
    b. window.location.href = "/"（跳回应用首页）

14. login/index.vue 的 onMounted 检测到 casdoor_auth_token
15. 调用 fetchMe() 填充 userName
16. 跳转到面板页
```

### 请求认证

```
1. 前端每次请求带 X-User-Token: <casdoor_auth_token> header
2. request.ts 的 getToken() 优先读 casdoor_auth_token，其次读 pc_user_token
3. requireUser() 中间件拿到 token，调用 sidecar POST /api/auth/verify
4. sidecar 查本地 session，如果 JWT 快过期自动 refresh
5. 返回用户 identity，pinhaoclaw 调 findOrCreateSidecarUser 创建/更新本地用户
```

### 退出登录流程

退出登录需要处理的关键问题是：**Casdoor 的 session cookie 在 Casdoor 域（例如 `auth.example.com`）下，且标记为 HttpOnly + SameSite=None + Secure。由于浏览器安全策略，业务站点无法直接清除 Casdoor 域下的 cookie。**

因此退出策略是：**销毁本地 session + 下次登录时强制重新认证**。

```
1. 用户点击"退出登录"（panel/index.vue 的 el-dropdown-item）
2. userStore.logout() 执行:
   a. 若当前是 sidecar 登录态，先调用 `POST /api/auth/sidecar/logout` 销毁 sidecar session
   b. 清除 localStorage 所有认证相关 key:
      - pc_user_token
      - pc_user_id
      - pc_user_name
      - pc_max_lobsters
      - casdoor_auth_token
   c. 设置 force_relogin = "1"（标记用户主动退出）
   d. `uni.reLaunch({ url: "/pages/login/index" })` 回到登录页

3. 前端后续回到登录页，再次点击统一认证入口
4. `goSidecarLogin()` 检测到 `force_relogin` 标记
5. 在 login_url 后追加 `?prompt=login`，然后删除 `force_relogin` 标记

6. Casdoor 收到 prompt=login 参数，即使浏览器有 Casdoor session cookie，
   也会强制显示登录表单，要求用户重新输入密码
```

**为什么不直接清除 Casdoor cookie？**

| 方法 | 为什么不可行 |
|------|------------|
| `fetch` + `credentials:include` POST 到 Casdoor `/api/logout` | Casdoor 不支持 CORS，preflight 返回 403 |
| 隐藏 iframe 加载 Casdoor `/logout` 页面 | Chrome 默认阻止第三方 iframe 中的 cookie |
| 隐藏 form POST 到 Casdoor `/api/logout` | iframe 中 Secure cookie 在 HTTP 页面不发送 |
| `window.location.href` 直接跳转 Casdoor `/logout` | Casdoor 登出后停在 Casdoor 登录页，不跳回我们的应用 |
| 后端代理调 Casdoor `/api/logout` | Casdoor session cookie 在浏览器端，后端请求不带 cookie |

**`prompt=login` 方案的优点**：
- 不需要清除 Casdoor cookie，只需要让 Casdoor 不自动授权
- 标准 OAuth2/OIDC 参数，Casdoor 原生支持
- 用户体验一致：退出后必须重新输入密码

## 切换访问地址时需要改的配置

如果 pinhaoclaw 的访问地址变了（比如从测试域名切到正式域名），需要改 4 处：

1. **sidecar** `config.yaml` → `server.public_origin`
2. **pinhaoclaw** `.env` → `PINHAOCLAW_PUBLIC_ORIGIN`
3. **pinhaoclaw** `.env` → `PINHAOCLAW_CASDOOR_ENDPOINT`（如果 Casdoor 地址也变了）
4. **Casdoor** 应用的 `redirect_uris` 添加新回调地址

改完后需要重启 sidecar 和 pinhaoclaw。如果改了 Casdoor 的 redirect_uris，还需要重启 Casdoor（或等缓存过期）。

## 注意事项

- sidecar session 有效期 7 天（无访问自动过期），JWT 过期前 5 分钟自动续期
- Casdoor 的 `grantTypes` 字段不能为空数组或 null，否则所有登录都会失败
- Casdoor 的 `redirect_uris` 中每个 URL 必须完全匹配（包括端口号）
- sidecar 只监听 localhost，外部访问必须通过 pinhaoclaw 代理
- `force_relogin` 标记存在 localStorage，只在用户主动退出时设置，登录成功后自动清除
- 如果用户关闭浏览器标签但不点退出，再次打开时 Casdoor 可能自动授权（无需输入密码）——这是预期行为
