# 拼好虾 PinHaoClaw

> 隔离式 AI 虾座 SaaS 管理平台 — 用户购买专属 AI 助手「龙虾」，每只龙虾绑定独立微信账号，部署在云端节点上。

## 核心概念

| 角色 | 说明 |
|------|------|
| **用户** | 通过邀请码登录，购买/管理自己的龙虾，查看 Token 和空间用量 |
| **龙虾 (Lobster)** | 一个独立的 picoclaw 实例，绑定用户微信，有独立的端口、Token 配额(100w/月) 和空间配额(2GB/月) |
| **节点 (Node)** | 云服务器，可部署多只龙虾，标记区域标签（华南/华北/华东/华中/境外） |
| **管理员** | 通过隐藏入口管理节点、邀请码、全局设置 |

## 项目结构

```
pinhaoclaw/
├── main.go                  # 程序入口
├── go.mod / go.sum
├── config/
│   └── config.go            # 环境变量配置（AdminPassword、RemoteRegion 等）
├── sharing/
│   └── store.go             # JSON 文件数据模型（User/Lobster/Node/Invite/Settings）
├── claw/
│   ├── service.go           # NodeService：SSH 远程部署、实例管理、微信绑定
│   └── backend/
│       └── ssh_client.go    # SSH 客户端封装（StreamRun PTY 流式执行）
├── server/
│   ├── app.go               # Gin 路由 + 全部 REST Handler + 静态文件托管
│   ├── auth.go              # 用户邀请码认证 + 管理员 Token 认证
│   └── ws.go                # WebSocket Handler（小程序端微信绑定推送）
│
└── pinhaoclaw-frontend/     # uni-app Vue3 跨平台前端
    ├── src/
    │   ├── pages/
    │   │   ├── login/       # 邀请码登录页
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

> 后端自动从 `pinhaoclaw-frontend/dist/build/h5/` 托管 H5 产物，访问 `http://<host>:9000` 即可。

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
./pinhaoclaw -debug            # Debug 模式
```

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | 监听端口 | `9000` |
| `PINHAOCLAW_ADMIN_PASSWORD` | 管理员密码（必须设置） | （空） |
| `PINHAOCLAW_REMOTE_HOST` | 默认节点地址（可选） | （空） |
| `PINHAOCLAW_REMOTE_SSH_PORT` | 默认节点 SSH 端口 | `22` |
| `PINHAOCLAW_REMOTE_USER` | SSH 用户 | `root` |
| `PINHAOCLAW_REMOTE_PASSWORD` | SSH 密码 | （空） |
| `PINHAOCLAW_REMOTE_REGION` | 默认节点区域标签 | `华南` |

## API 接口

### 用户认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/auth/login` | 邀请码登录 |
| GET | `/api/auth/me` | 当前用户信息 |

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
| — | `{AdminPath}` | 管理面板页面 |
| GET/POST/DEL | `/api/admin/invites` | 邀请码 CRUD |
| GET/POST/PUT/DEL | `/api/admin/nodes` | 节点 CRUD |
| GET | `/api/admin/lobsters` | 全部龙虾列表 |
| PUT | `/api/admin/settings` | 全局设置（默认配额等） |
| GET | `/api/regions` | 可用区域列表 |

## 核心流程

1. **管理员创建邀请码** → 设置最大使用次数
2. **用户邀请码登录** → 进入面板
3. **购买龙虾** → 选择区域 → 自动分配节点 → SSH 远程部署 picoclaw 实例
4. **绑定微信** → SSE 流式推送 QR 码 → 手机扫码登录
5. **用量管理** → Token(100w/月) + Space(2GB/月) 配额，每月自动重置

## 技术栈

- **Web**: [Gin](https://github.com/gin-gonic/gin) — 高性能 HTTP 框架
- **前端**: 内嵌 HTML + 原生 JS，单二进制部署，零外部依赖
- **存储**: JSON 文件 (`~/.pinhaoclaw/*.json`)，`sync.RWMutex` 并发安全
- **SSH**: sshpass + OpenSSH，支持密码模式和密钥模式
- **SSE**: Server-Sent Events 用于微信绑定流式推送
- **二维码**: [`github.com/skip2/go-qrcode`](https://github.com/skip2/go-qrcode) 服务端 PNG 生成
- **部署**: systemd service，交叉编译 Linux amd64

## 安全设计

- 管理后台使用随机路径 + 强密码保护
- Token 仅通过 HTTP Header 传递（不接受 Query 参数）
- 空密码一律拒绝登录
- SSH 连接超时控制，StreamRun 6 分钟上限
- 微信绑定 QR 码 URL 一次性服务端渲染

## License

MIT
