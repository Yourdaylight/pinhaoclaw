# 拼好虾 PinHaoClaw

> 隔离式 AI 龙虾 SaaS 管理平台 — 用户购买专属 AI 助手「龙虾」，每只龙虾绑定独立微信账号，部署在云端节点上。

## 核心概念

| 角色 | 说明 |
|------|------|
| **用户** | 通过邀请码或 sidecar 统一认证登录，购买/管理自己的龙虾，查看 Token 和空间用量 |
| **龙虾 (Lobster)** | 一个独立的 picoclaw 实例，绑定用户微信，有独立的端口、Token 配额(100w/月) 和空间配额(2GB/月) |
| **节点 (Node)** | 云服务器，可部署多只龙虾，标记区域标签（华南/华北/华东/华中/境外） |
| **管理员** | 通过隐藏入口管理节点、邀请码、全局设置 |

## 认证模式

项目支持两种认证模式，通过 `PINHAOCLAW_AUTH_MODE` 环境变量切换：

| 模式 | 值 | 说明 |
|------|-----|------|
| **邀请码** | `invite` | 原始模式：用户输入管理员发放的邀请码即可登录注册 |
| **Sidecar 统一认证** | `sidecar` | 通过 `casdoor-auth-sidecar` 代理统一认证；PinHaoClaw 负责登录转发、回调桥接和 token 校验 |
| **自动检测** | `auto`（默认） | 若配置了 `PINHAOCLAW_AUTH_SIDECAR_URL` 则走 sidecar，否则 fallback 到邀请码模式 |

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
│   ├── sidecar.go           # Sidecar 登录/回调代理、本地用户映射、统一认证桥接
│   └── ws.go                # WebSocket Handler（小程序端微信绑定推送）
│
└── pinhaoclaw-frontend/     # uni-app Vue3 跨平台前端
    ├── src/
    │   ├── pages/
    │   │   ├── login/       # 登录页（邀请码 / sidecar 双模式自适应）
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
1. 配置完整性（认证模式、URL 合法性、sidecar 必填项）
2. `PINHAOCLAW_HOME` 数据目录可写
3. 前端 `index.html` 存在

任一环节失败直接退出并打印原因。

## 测试与验证现状

当前仓库已经具备可执行的最小测试闭环：

- 后端：`go test ./...` 通过
- 后端构建：`go build ./...` 通过
- 前端：`npm ci && npm test && npm run build:h5` 通过
- 启动冒烟：可在临时数据目录下完成后端启动和 `/health` 检查

仍需注意两点：

- 当前前端测试是最小页面级测试，已覆盖登录页关键分支，但整体覆盖率仍偏低
- README 曾长期保留 Casdoor 直连表述，实际实现已经切到 sidecar 统一认证；本文档以下内容已按真实代码修正

---

## 启动配置（环境变量）

### 认证与前端

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | 监听端口 | `9000` |
| `PINHAOCLAW_HOME` | 运行时数据目录（用户/龙虾/邀请码等 JSON） | `~/.pinhaoclaw` |
| `PINHAOCLAW_FRONTEND_DIR` | H5 前端构建产物目录 | `pinhaoclaw-frontend/dist/build/h5` |
| `PINHAOCLAW_AUTH_MODE` | 认证模式：`invite` / `sidecar` / `auto` | `auto` |
| `PINHAOCLAW_PUBLIC_ORIGIN` | 对外访问地址，用于生成统一认证回调 URL | `http://localhost:9000` |

### Sidecar 统一认证

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PINHAOCLAW_AUTH_SIDECAR_URL` | sidecar 服务地址；设置后 `auto` 会自动切换到 sidecar 模式 | （空） |
| `PINHAOCLAW_CASDOOR_ENDPOINT` | Casdoor 外部地址；用于前端退出登录跳转 | （空） |

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
| GET | `/api/auth/config` | 获取当前认证模式与统一认证入口信息 | 全部 |
| POST | `/api/auth/login` | 邀请码登录 | `invite` |
| GET | `/api/auth/sidecar/login` | 重定向到 sidecar 登录入口 | `sidecar` |
| GET | `/api/auth/sidecar/callback` | sidecar 回调代理（code/state 原样透传） | `sidecar` |
| GET | `/api/auth/sidecar/logout` | 注销 sidecar 会话并跳回登录页 | `sidecar` |
| GET | `/api/auth/me` | 当前登录用户信息 | 全部 |

> `sidecar` 模式下调用 `/api/auth/login` 会返回 400，并提示走统一认证入口。

### 龙虾管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/lobsters` | 我的龙虾列表 |
| POST | `/api/lobsters` | 购买新龙虾（选区域→自动分配节点） |
| GET | `/api/lobsters/:id` | 龙虾详情 |
| GET | `/api/lobsters/:id/bind` | **SSE** 绑定微信（QR 码扫码） |
| POST | `/api/lobsters/:id/start` | 启动龙虾 |
| POST | `/api/lobsters/:id/stop` | 停止龙虾 |
| DELETE | `/api/lobsters/:id` | 删除龙虾 |
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

### sidecar（统一认证模式）

1. 用户访问登录页 → 点击统一认证入口，跳到 `/api/auth/sidecar/login`
2. Sidecar 生成 state 并重定向到 Casdoor 完成认证
3. Casdoor 回调进入 `/api/auth/sidecar/callback` → PinHaoClaw 透传给 sidecar 完成会话桥接
4. 业务请求携带 token → 后端通过 sidecar `/api/auth/verify` 校验身份，并按 `sub` 映射/创建本地用户
4. 进入控制台 → （后续步骤同上）

## 技术栈

| 层 | 技术选型 |
|---|---|
| **后端框架** | [Gin](https://github.com/gin-gonic/gin) |
| **前端** | [uni-app](https://uniapp.net/) Vue3 + Element Plus (H5) / 原生组件 (小程序) |
| **存储** | JSON 文件 (`PINHAOCLAW_HOME/*.json`)，`sync.RWMutex` 并发安全 |
| **身份认证** | 邀请码（内置）/ sidecar 统一认证 / 本地用户映射 |
| **远程部署** | SSH（密码/密钥双模式），StreamRun PTY 流式执行 |
| **实时推送** | SSE (H5 微信绑定) / WebSocket (小程序微信绑定) |
| **二维码** | [`github.com/skip2/go-qrcode`](https://github.com/skip2/go-qrcode) 服务端 PNG 生成 |
| **部署形态** | 单二进制 + 静态文件内嵌，systemd 托管 |

## 提交质量门禁

仓库内已提供一套可版本化的 commit / push / PR 质量门禁：

1. 安装本地 hooks

```bash
./tools/setup-git-hooks.sh
```

2. 本地快速检查（适合提交前手动跑）

```bash
./tools/quality-gate.sh commit
```

3. 本地完整检查（等价于 pre-push / PR）

```bash
./tools/quality-gate.sh push
```

门禁规则如下：

- `commit-msg`：强制 Conventional Commits，限制标题长度，拦截不可审计的提交信息
- `pre-push`：执行 Go 格式检查、Go 测试、Go 构建、前端页面测试、前端 H5 构建、启动冒烟检查
- GitHub Actions PR 校验：在干净环境中执行同一套 `push` 级校验，避免“我本地能过”

## 安全设计

- **启动 fail-fast**：关键配置缺失、前端产物不存在、数据目录不可写均直接退出
- **管理后台**：随机隐藏路径 + 密码保护；未设密码时登录接口直接禁用
- **Token 安全**：仅通过 HTTP Header (`X-User-Token` / `X-Admin-Token`) 传递，拒绝 Query 参数注入
- **路径隔离**：`/scripts`、`/.git`、`.env`、源码文件等敏感路径被显式拦截返回 404，不回退到 SPA index.html
- **统一认证隔离**：OAuth state、token 交换和会话管理下沉到 sidecar，业务服务只做入口代理和 token 校验
- **SSH 安全**：连接超时控制，StreamRun 6 分钟上限；QR 码 URL 服务端一次性渲染

## License

MIT
