# Casdoor + casdoor-auth-sidecar 统一认证接入最佳实践

本文档是当前 PinHaoClaw 接入 Casdoor 统一认证的最佳实践总结，目标不是解释所有实现细节，而是给出一套稳定、可复用、可上线的接入方法。

如果你要在新的业务系统里复用这套方案，优先遵循本文档。详细实现细节可再参考现有集成文档。

## 1. 目标架构

推荐架构如下：

```text
Browser
  |
  | 1. GET /api/auth/sidecar/login
  v
Business App (public)
  |
  | proxy
  v
casdoor-auth-sidecar (private, localhost/internal only)
  |
  | OAuth2 / OIDC
  v
Casdoor
```

关键原则：

- 浏览器永远不要直接访问 sidecar 的内网地址。
- sidecar 只负责统一认证，不直接承载业务页面。
- 业务应用永远是浏览器看到的唯一入口。
- Casdoor 的 OAuth2 细节、session 管理、refresh token 续期都放在 sidecar 内处理。

## 2. 角色分工

### Casdoor

- 负责用户登录、统一身份、OAuth2/OIDC token 发放。
- 负责 Casdoor 域名下的浏览器 session cookie。
- 负责最终的统一认证登出。

### casdoor-auth-sidecar

- 负责 OAuth2 授权码流程。
- 负责把 Casdoor token 换成业务可用的 sidecar session token。
- 负责 session 持久化和自动续期。
- 负责把 Casdoor 身份转换成统一的身份对象给业务后端使用。

### Business App

- 负责代理 sidecar 的 login、callback、logout、logout-complete 路由。
- 负责基于 sidecar identity 建立或更新本地用户。
- 负责所有业务 API。
- 负责给前端暴露统一的登录入口和退出入口。

### Frontend

- 只认业务应用暴露出来的统一认证入口。
- 持有 sidecar session token，不直接持有 Casdoor cookie。
- 收到 401 时回到业务登录页，不自己碰 OAuth2 细节。

## 3. 推荐登录链路

推荐链路：标准授权码模式 + 业务代理 + sidecar session。

```text
1. Browser -> GET /api/auth/sidecar/login
2. App proxy -> sidecar /api/auth/login
3. sidecar -> 302 to Casdoor /login/oauth/authorize
4. 用户在 Casdoor 登录
5. Casdoor -> redirect to /api/auth/sidecar/callback
6. App proxy -> sidecar /api/auth/callback
7. sidecar exchange code -> create local session
8. sidecar 返回 bridge HTML
9. Browser 写入 casdoor_auth_token(localStorage)
10. Frontend 后续请求带 X-User-Token
11. App -> sidecar /api/auth/verify
```

最佳实践：

- sidecar 的 redirect URI 必须回到业务应用代理路由，不要直接回 sidecar 原始地址。
- 前端只跳 `/api/auth/sidecar/login`，不要自己拼 Casdoor authorize URL。
- 后端认证中间件统一调用 sidecar `/api/auth/verify`，不要业务系统自己验 Casdoor JWT。
- 使用 sidecar session token 作为前端持有 token，不要把 Casdoor access token 直接下放给业务前端。

## 4. 推荐登出链路

这是最容易做错的部分。

### 正确做法

正确目标不是“删本地 token 就算退出”，而是同时满足两件事：

- 业务本地登录态清掉。
- Casdoor 域名下的浏览器登录态也真正清掉。

当前最佳实践链路：

```text
1. 用户点击退出登录
2. Browser -> POST /api/auth/sidecar/logout
3. Business App proxy -> sidecar /api/auth/logout
4. sidecar 删除自己的 session
5. 前端清理本地 token / 用户信息
6. 前端设置 force_relogin 标记并回到登录页
7. 用户下次再次点击统一认证入口时，前端追加 ?prompt=login
8. Casdoor 即使仍有浏览器 cookie，也会强制显示登录表单
```

### 为什么这样做

原因很直接：

- Casdoor cookie 在 Casdoor 域名下，业务域名删不掉。
- 后端服务端请求 Casdoor `/api/logout` 不会带浏览器 cookie，不能替代浏览器真实登出。
- 如果把主窗口直接导航到 Casdoor，用户会看到 auth 页，体验差。
- 如果直接从业务前端跨域 fetch Casdoor `/api/logout`，通常会遇到 CORS、cookie、网关策略问题。

### 为什么当前实现不再直接走 Casdoor 前端登出页

当前实现已经收敛为：优先销毁业务本地 session，并通过 `force_relogin + prompt=login` 强制下次重新输入凭据。

这样做的原因是：

- 避免把业务 token 暴露到 URL。
- 避免把浏览器主窗口导航到认证域名。
- 避免依赖跨域 iframe / CORS / 网关行为。
- 对业务系统而言，效果仍然是“退出后必须重新登录”。

## 5. 最推荐的前端策略

前端只做 4 件事：

1. 读取 `/api/auth/config` 判断当前是否启用 sidecar。
2. 点击登录时跳业务应用的 `/api/auth/sidecar/login`。
3. 登录成功后持有 sidecar session token。
4. 点击退出时跳业务应用的 `/api/auth/sidecar/logout`。

前端不要做的事：

- 不要自己直连 Casdoor `/api/logout`。
- 不要自己拼 OAuth2 回调参数。
- 不要自己解析 Casdoor JWT 作为权限依据。
- 不要把 Casdoor endpoint 当成业务前端路由去硬跳。

## 6. 最推荐的后端策略

业务后端统一做这几件事：

- 暴露 `/api/auth/sidecar/login`
- 暴露 `/api/auth/sidecar/callback`
- 暴露 `/api/auth/sidecar/logout`
- 暴露 `/api/auth/sidecar/logout-complete`
- 所有受保护 API 统一走 `sidecar /api/auth/verify`

不要做的事：

- 不要在业务服务里复制一套 OAuth2 code exchange 逻辑。
- 不要在多个业务服务里各自维护 Casdoor refresh token。
- 不要让浏览器直接访问 sidecar 原始地址。

## 7. 配置最佳实践

### sidecar 配置

核心字段：

```yaml
casdoor:
  endpoint: https://auth.example.com
  client_id: your-client-id
  client_secret: your-client-secret
  organization: your-org
  application: your-app
  redirect_path: /api/auth/sidecar/callback
server:
  listen: :9098
  public_origin: https://your-app.example.com
```

规则：

- `public_origin` 必须是用户真实访问业务应用的外部地址。
- `redirect_path` 应该是业务应用上的代理回调地址。
- `organization` 和 `application` 不能缺，登出路由需要它们来拼 Casdoor 前端 logout URL。

### Casdoor 应用配置

必须保证：

- `redirect_uris` 包含 `https://your-app.example.com/api/auth/sidecar/callback`
- 应用的 `organization`、`application` 名称与你 sidecar 配置一致
- 线上环境必须优先使用 HTTPS

## 8. 部署最佳实践

### 网络边界

- Casdoor 对外可访问。
- Business App 对外可访问。
- sidecar 仅内网或 localhost 可访问。

### 反向代理

- 反向代理只代理业务应用。
- 业务应用再代理 sidecar 认证路由。
- 不要把 sidecar 直接暴露到公网。

### HTTPS

- 统一认证的外部入口应使用 HTTPS。
- 如果业务站点和 Casdoor 混用 HTTP/HTTPS，浏览器 cookie 行为会复杂化，登出链路很容易出问题。

## 9. 常见错误与规避方式

### 错误 1：浏览器直跳 sidecar

问题：

- 暴露 sidecar 到公网
- 业务域名和回调域名分裂
- 浏览器和业务系统都绕不过 sidecar 地址

规避：

- 只让业务应用对外
- sidecar 永远走代理

### 错误 2：把 Casdoor `/api/logout` 当成普通后端 API 去调

问题：

- 服务端请求不带浏览器 cookie
- 实际不会清掉浏览器 Casdoor 登录态

规避：

- 让浏览器在 Casdoor 同源上下文内完成登出
- 推荐使用 Casdoor 前端 logout 页面承接

### 错误 3：直接导航主窗口到 Casdoor 登出页

问题：

- 用户会看到 auth 页面
- 用户体验差

规避：

- 用隐藏 iframe 或新窗口后台完成 Casdoor 登出
- 主窗口只负责业务页跳转

### 错误 4：用错 Casdoor 前端登出路由

问题：

- 根路径 `/logout` 可能 404
- 不会触发 Casdoor 的前端登出组件

规避：

- 先按当前 Casdoor 前端实际路由确认
- 当前推荐用 `/cas/{organization}/{application}/logout`

### 错误 5：只靠 `prompt=login` 模拟退出

问题：

- 这只能降低自动登录概率，不能等价于真正统一认证登出
- 从“统一认证”角度看，用户其实还留在 Casdoor 会话里

规避：

- `prompt=login` 只能当兜底，不应该替代 Casdoor 真登出

## 10. 推荐接入清单

新系统接入时，按这个 checklist 走：

1. 业务系统只暴露统一认证代理路由，不暴露 sidecar。
2. sidecar 配置好 `endpoint/client_id/client_secret/organization/application/public_origin`。
3. Casdoor 应用配置好 callback redirect URI。
4. 业务前端只跳 `/api/auth/sidecar/login`。
5. 业务后端认证中间件统一调用 sidecar `/api/auth/verify`。
6. 登出采用“本地清理 + 隐藏 iframe + Casdoor 前端 logout + logout-complete 回调”。
7. 线上环境统一走 HTTPS。
8. 验证登录、刷新、401 失效、退出后重新登录四条链路。

## 11. 当前 PinHaoClaw 的推荐结论

对于 PinHaoClaw，目前推荐做法已经收敛为：

- 登录：`/api/auth/sidecar/login` -> sidecar -> Casdoor OAuth2
- 校验：业务后端统一调用 sidecar `/api/auth/verify`
- 登出：业务页面内隐藏 iframe 打开 Casdoor 的 `/cas/JQ/app_pinhaoclaw_jq/logout?service=...`
- 回跳：Casdoor -> `/api/auth/sidecar/logout-complete` -> iframe postMessage -> 主窗口返回业务首页

这套方式的优点：

- 用户只看到业务站点，不看到 auth 登出页
- Casdoor 浏览器登录态能真正清掉
- 业务系统不需要自己处理 OAuth2 细节
- sidecar 继续保持统一认证边界

## 12. 一句话原则

一句话总结：

**登录走业务代理，认证走 sidecar session，登出让 Casdoor 在自己的域名里完成，但不要让用户看到 Casdoor 页面。**