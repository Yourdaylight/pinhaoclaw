# 拼好虾 PinHaoClaw

> 隔离式 AI 龙虾 SaaS 管理平台 — 用户购买专属 AI 助手「龙虾」，每只龙虾绑定独立微信账号，部署在云端节点上。

## 核心概念

| 角色 | 说明 |
|------|------|
| **用户** | 通过邀请码或 Casdoor 统一认证登录，购买/管理自己的龙虾，查看 Token 和空间用量 |
| **龙虾 (Lobster)** | 一个独立的 picoclaw 实例，绑定用户微信，有独立的端口、Token 配额(100w/月) 和空间配额(2GB/月) |
| **节点 (Node)** | 云服务器，可部署多只龙虾，标记区域标签（华南/华北/华东/华中/境外） |
| **管理员** | 通过隐藏入口管理节点、邀请码、全局设置 |

## 认证模式

项目支持两种认证模式，通过 `PINHAOCLAW_AUTH_MODE` 环境变量切换：

| 模式 | 值 | 说明 |
|------|-----|------|
| **邀请码** | `invite` | 原始模式：用户输入管理员发放的邀请码即可登录注册 |
| **Casdoor 统一认证** | `casdoor` | 接入 [Casdoor](https://casdoor.org) 身份平台，用户在统一认证中心完成登录/注册（支持邮箱验证码+图形验证码），通过 OAuth 回调进入系统 |
| **自动检测** | `auto`（默认） | 若配置了 `PINHAOCLAW_CASDOOR_*` 相关环境变量则走 Casdoor，否则 fallback 到邀请码模式 |

> 两种模式下前端会根据 `/api/auth/config` 返回的 `mode` 自动渲染对应的登录页面。

## 项目结构

```
pinhaoclaw/
├── main.go                  # 程序入口，启动配置加载与 fail-fast 校验
├── go.mod / go.sum
├── config/
│   └── config.go            # 启动配置模型、校验逻辑、AuthMode 解析
├── sharing/
│   └── store.go             # JSON 文件数据模型（User/Lobster/Node/Invite/Settings）
├── claw/
│   ├── service.go           # NodeService：SSH 远程部署、实例管理、微信绑定
│   └── backend/
│       └── ssh_client.go    # SSH 客户端封装（StreamRun PTY 流式执行）
├── server/
│   ├── app.go               # Gin 路由 + 全部 Handler + 静态文件托管 + 敏感路径拦截
│   ├── auth.go              # 邀请码认证 + 管理员 Token 认证
│   ├── casdoor.go           # Casdoor OAuth 登录/回调/用户映射/登录桥接页
│   └── ws.go                # WebSocket Handler（小程序端微信绑定推送）
│
└── pinhaoclaw-frontend/     # uni-app Vue3 跨平台前端
    ├── src/
    │   ├── pages/
    │   │   ├── login/       # 登录页（邀请码 / Casdoor 双模式自适应）
    │   │   ├── panel/       # 用户龙虾面板
    │   │   ├── lobster/     # 绑定微信页（SSE/WebSocket 条件编译）
    │   │   └── admin/       # 管理后台（#ifdef H5 仅 Web 端）
    │   ├── components/      # LobsterCard / QuotaBar 等组件
    │   ├── stores/          # Pinia：user / lobster
    │   └── api/             # 请求封装：auth / lobster / admin
    └── dist/build/h5/       # H5 构建产物（后端静态托管目录）
```

## 快速开始

### 前端构建（H5）

```bash
cd pinhaoclaw-frontend
npm install
npm run build:h5        # 产物输出到 dist/build/h5/
```

### 后端编译

```bash
cd pinhaoclaw
go mod tidy

# 本地开发
go run main.go

# 交叉编译 Linux amd64
GOOS=linux GOARCH=amd64 go build -o pinhaoclaw .
```

> 后端通过 `PINHAOCLAW_FRONTEND_DIR` 指定 H5 产物目录，启动时会校验 `index.html` 是否存在；缺失则 fail-fast。

### 微信小程序

```bash
cd pinhaoclaw-frontend
# 修改 src/api/request.ts 中的 BASE_URL 为实际后端地址（HTTPS）
npm run build:mp-weixin   # 产物在 dist/build/mp-weixin/
# 使用微信开发者工具导入产物目录
```

### 运行

```bash
./pinhaoclaw                    # 默认 :9000
./pinhaoclaw -p 8080           # 自定义端口
```

启动时会依次检查：
1. 配置完整性（认证模式、URL 合法性、Casdoor 必填项）
2. `PINHAOCLAW_HOME` 数据目录可写
3. 前端 `index.html` 存在

任一环节失败直接退出并打印原因。

---

## 启动配置（环境变量）

### 认证与前端

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | 监听端口 | `9000` |
| `PINHAOCLAW_HOME` | 运行时数据目录（用户/龙虾/邀请码等 JSON） | `~/.pinhaoclaw` |
| `PINHAOCLAW_FRONTEND_DIR` | H5 前端构建产物目录 | `pinhaoclaw-frontend/dist/build/h5` |
| `PINHAOCLAW_AUTH_MODE` | 认证模式：`invite` / `casdoor` / `auto` | `auto` |
| `PINHAOCLAW_PUBLIC_ORIGIN` | 对外访问地址，用于生成 Casdoor OAuth 回调 URL | `http://localhost:9000` |

### Casdoor 统一认证（`casdoor` 模式必填）

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PINHAOCLAW_CASDOOR_ENDPOINT` | Casdoor 服务地址 | （空） |
| `PINHAOCLAW_CASDOOR_CLIENT_ID` | Casdoor Application Client ID | （空） |
| `PINHAOCLAW_CASDOOR_CLIENT_SECRET` | Casdoor Application Client Secret | （空） |
| `PINHAOCLAW_CASDOOR_ORGANIZATION` | Casdoor 组织名 | `JQ` |
| `PINHAOCLAW_CASDOOR_APPLICATION` | Casdoor 应用名 | `app_pinhaoclaw_jq` |
| `PINHAOCLAW_CASDOOR_REDIRECT_PATH` | OAuth 回调路径 | `/api/auth/callback` |

### 管理后台

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PINHAOCLAW_ADMIN_PASSWORD` | 管理员密码；为空时管理后台登录**被禁用** | （空） |
| `PINHAOCLAW_ADMIN_PATH` | 管理后台隐藏路径；为空时自动随机生成（如 `/mgr-x8kP2mQa`） | （空） |

### 远程节点部署

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PINHAOCLAW_REMOTE_HOST` | 默认节点地址（可选） | （空） |
| `PINHAOCLAW_REMOTE_SSH_PORT` | 默认节点 SSH 端口 | `22` |
| `PINHAOCLAW_REMOTE_USER` | SSH 用户 | `root` |
| `PINHAOCLAW_REMOTE_KEY_PATH` | SSH 私钥路径（优先于密码） | （空） |
| `PINHAOCLAW_REMOTE_PASSWORD` | SSH 密码 | （空） |
| `PINHAOCLAW_REMOTE_HOME` | 远程部署根目录 | `/opt/pinhaoclaw` |
| `PINHAOCLAW_REMOTE_REGION` | 默认节点区域标签 | `华南` |

> 本地运维/部署脚本统一放在仓库内 `scripts/` 目录，该目录已加入 `.gitignore`，**默认不纳入版本控制**。

## API 接口

### 认证配置 & 登录

| 方法 | 路径 | 说明 | 适用模式 |
|------|------|------|----------|
| GET | `/api/auth/config` | 获取当前认证模式与 Casdoor 配置信息 | 全部 |
| POST | `/api/auth/login` | 邀请码登录 | `invite` |
| GET | `/api/auth/login/casdoor` | 重定向到 Casdoor OAuth 授权页 | `casdoor` |
| GET | `/api/auth/callback` | Casdoor OAuth 回调（code→token→本地用户） | `casdoor` |
| GET | `/api/auth/me` | 当前登录用户信息 | 全部 |

> `casdoor` 模式下调用 `/api/auth/login` 会返回 400 并提示走统一认证入口。

### 龙虾管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/lobsters` | 我的龙虾列表 |
| POST | `/api/lobsters` | 购买新龙虾（选区域→自动分配节点） |
| GET | `/api/lobsters/:id` | 龙虾详情 |
| GET | `/api/lobsters/:id/bind` | **SSE** 绑定微信（QR 码扫码） |
| POST | `/api/lobsters/:id/start` | 启动龙虾 |
| DELETE | `/api/lobsters/:id/stop` | 停止龙虾 |
| GET | `/api/qrcode?url=` | 二维码图片生成 |

### 管理后台

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/admin/login` | 管理员登录获取 token |
| — | `{AdminPath}` | 管理面板 SPA 页面 |
| GET/POST/DEL | `/api/admin/invites` | 邀请码 CRUD |
| GET/POST/PUT/DEL | `/api/admin/nodes` | 节点 CRUD |
| GET | `/api/admin/lobsters` | 全部龙虾列表 |
| PUT | `/api/admin/settings` | 全局设置（默认配额等） |
| GET | `/api/regions` | 可用区域列表 |

## 核心流程

### invite（邀请码模式）

1. 用户访问登录页 → 输入管理员发放的邀请码
2. 后端校验邀请码有效性 / 使用次数 → 创建或复用本地用户
3. 进入控制台 → 购买龙虾 → 系统按区域自动分配节点并 SSH 部署
4. 绑定微信 → SSE / WebSocket 流式推送 QR 码 → 扫码完成绑定
5. 用量管理 → Token(100w/月) + Space(2GB/月) 配额，每月自动重置

### casdoor（统一认证模式）

1. 用户访问登录页 → 点击「前往统一认证中心」跳转 Casdoor OAuth
2. 在 Casdoor 完成注册/登录（邮箱验证码 + 图形验证码）
3. OAuth 回调到 PinHaoClaw → 后端换 token → 按 Casdoor sub 映射/创建本地用户
4. 进入控制台 → （后续步骤同上）

## 技术栈

| 层 | 技术选型 |
|---|---|
| **后端框架** | [Gin](https://github.com/gin-gonic/gin) |
| **前端** | [uni-app](https://uniapp.net/) Vue3 + Element Plus (H5) / 原生组件 (小程序) |
| **存储** | JSON 文件 (`PINHAOCLAW_HOME/*.json`)，`sync.RWMutex` 并发安全 |
| **身份认证** | 邀请码（内置）/ Casdoor OAuth2 / OIDC JWT 解析 |
| **远程部署** | SSH（密码/密钥双模式），StreamRun PTY 流式执行 |
| **实时推送** | SSE (H5 微信绑定) / WebSocket (小程序微信绑定) |
| **二维码** | [`github.com/skip2/go-qrcode`](https://github.com/skip2/go-qrcode) 服务端 PNG 生成 |
| **部署形态** | 单二进制 + 静态文件内嵌，systemd 托管 |

## 安全设计

- **启动 fail-fast**：关键配置缺失、前端产物不存在、数据目录不可写均直接退出
- **管理后台**：随机隐藏路径 + 密码保护；未设密码时登录接口直接禁用
- **Token 安全**：仅通过 HTTP Header (`X-User-Token` / `X-Admin-Token`) 传递，拒绝 Query 参数注入
- **路径隔离**：`/scripts`、`/.git`、`.env`、源码文件等敏感路径被显式拦截返回 404，不回退到 SPA index.html
- **OAuth State 防 CSRF**：Casdoor 登录携带一次性 state 参数，回调时严格校验
- **SSH 安全**：连接超时控制，StreamRun 6 分钟上限；QR 码 URL 服务端一次性渲染

## License

MIT
