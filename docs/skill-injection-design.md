# PinHaoClaw Skill 注入设计文档

## 概述

PinHaoClaw 平台需要维护一个 Skill 库，让用户可以一键将 Skill 安装到自己的龙虾（picoclaw 实例）中。本文档描述 Skill 注入的架构、API 和隔离机制。

## 隔离分析

### 当前已有的隔离机制

| 层面 | 隔离方式 | 实现位置 |
|------|---------|---------|
| **进程隔离** | 每只龙虾 = 独立 picoclaw gateway 进程，独立 PID | `claw/service.go` StartInstance |
| **文件系统隔离** | 每只龙虾独立目录 `{RemoteHome}/instances/{lobsterID}/` | `claw/service.go` CreateInstance |
| **网络隔离** | 每只龙虾独立端口 (8100-8300) | `claw/service.go` AllocatePort |
| **配额隔离** | Token/Space 配额按龙虾独立计算 | `sharing/store.go` Lobster |
| **用户数据隔离** | API 层 `l.UserID != u.ID` 校验 | `server/app.go` handlers |
| **ID 安全** | `validateSafeID()` 防路径穿越 | `claw/service.go` |

### Skill 隔离加固

**问题**：picoclaw 的 `SkillsLoader` 会加载 `~/.picoclaw/skills/` 全局目录。如果 `HOME` 指向用户真实 home，同一节点上所有龙虾会共享该目录。

**解决方案**：在 `StartInstance` 时设置 `HOME={instanceDir}`，使 `~/.picoclaw/skills` 被限定在龙虾实例目录内：

```bash
cd {workspace} && HOME={instDir} nohup picoclaw gateway --home . --port {port} ...
```

这样每只龙虾的 Skill 空间完全独立。

## 目录结构

```
{RemoteHome}/instances/{lobsterID}/
├── workspace/        # picoclaw 工作目录 (--home .)
├── skills/           # 龙虾已安装的 Skill（注入目标）
│   ├── firefly-iii-bookkeeper/
│   │   ├── SKILL.md
│   │   ├── scripts/
│   │   └── references/
│   └── rt-stock/
│       └── SKILL.md
├── logs/
│   └── gateway.log
├── picoclaw.pid
└── start.sh
```

## 数据模型

### SkillRegistryEntry（Skill 库条目，管理员维护）

```go
type SkillRegistryEntry struct {
    Slug        string         `json:"slug"`
    DisplayName string         `json:"display_name"`
    Summary     string         `json:"summary"`
    Category    string         `json:"category"`
    Author      string         `json:"author"`
    Version     string         `json:"version"`
    Icon        string         `json:"icon,omitempty"`
    Tags        []string       `json:"tags,omitempty"`
    Requires    *SkillRequires `json:"requires,omitempty"`
    Source      SkillSource    `json:"source"`
    IsVerified  bool           `json:"is_verified"`
    CreatedAt   string         `json:"created_at"`
    UpdatedAt   string         `json:"updated_at"`
}

type SkillSource struct {
    Type     string `json:"type"`               // "github" | "clawhub" | "local" | "builtin"
    Repo     string `json:"repo,omitempty"`      // GitHub "user/repo"
    ClawHub  string `json:"clawhub,omitempty"`   // ClawHub slug
    LocalDir string `json:"local_dir,omitempty"` // 本地目录
}

type SkillRequires struct {
    Bins []string `json:"bins,omitempty"` // ["python3", "node"]
    Env  []string `json:"env,omitempty"`  // ["FIREFLY_III_URL"]
}
```

### LobsterSkill（龙虾已安装记录）

```go
type LobsterSkill struct {
    Slug        string `json:"slug"`
    Version     string `json:"version"`
    InstalledAt string `json:"installed_at"`
}
```

## API 设计

### 用户端

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/skills` | 浏览 Skill 库 |
| GET | `/api/skills/:slug` | Skill 详情 |
| GET | `/api/lobsters/:id/skills` | 查看龙虾已安装 Skill |
| POST | `/api/lobsters/:id/skills` | 安装 Skill (body: `{"slug": "..."}`) |
| DELETE | `/api/lobsters/:id/skills/:slug` | 卸载 Skill |

### 管理员端

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/skills` | 列出所有 Skill 库条目 |
| POST | `/api/admin/skills` | 添加 Skill |
| PUT | `/api/admin/skills/:slug` | 更新 Skill |
| DELETE | `/api/admin/skills/:slug` | 删除 Skill |

## Skill 安装流程

### GitHub 类型 Skill

```
用户点击安装 → API 收到 {slug} → 查找 SkillRegistryEntry →
SSH 到远端节点 → git clone --depth 1 {repo} {instDir}/skills/{slug} →
rm -rf .git → 验证 SKILL.md 存在 → 记录 LobsterSkill
```

### Builtin 类型 Skill

```
用户点击安装 → API 收到 {slug} → 查找 SkillRegistryEntry →
SSH 到远端节点 → cp -r {localDir}/* {instDir}/skills/{slug}/ →
验证 SKILL.md 存在 → 记录 LobsterSkill
```

### ClawHub 类型 Skill

```
用户点击安装 → API 收到 {slug} → 查找 SkillRegistryEntry →
SSH 到远端节点 → picoclaw skill install --slug {slug} --target {dir} →
记录 LobsterSkill
```

## 管理员添加 Skill 示例

```bash
curl -X POST http://localhost:8080/api/admin/skills \
  -H "X-Admin-Token: xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "firefly-iii-bookkeeper",
    "display_name": "Firefly III 智能记账",
    "summary": "通过自然语言快速记账到 Firefly III",
    "category": "记账",
    "author": "鲸奇互联",
    "version": "1.0.0",
    "icon": "💰",
    "tags": ["记账", "Firefly", "财务"],
    "requires": {
      "bins": ["python3"],
      "env": ["FIREFLY_III_URL", "FIREFLY_III_API_KEY"]
    },
    "source": {
      "type": "github",
      "repo": "your-org/firefly-iii-bookkeeper"
    },
    "is_verified": true
  }'
```

## 存储设计

- `skill_registry.json` - Skill 库全局注册表 (map[slug]entry)
- `lobster_skills_{lobsterID}.json` - 每只龙虾的已安装 Skill 列表
- 使用 flock 文件锁保证并发安全
- 使用 tmp + rename 原子写入

## 安全说明

- Skill 库仅管理员可上传，无需额外安全检查
- `validateSafeID()` 防止路径穿越攻击
- `shellEscape()` 防止 shell 注入
- 用户只能操作自己的龙虾 (l.UserID != u.ID 校验)
- HOME 隔离确保不同龙虾的 Skill 空间完全独立

## 已修改文件清单

| 文件 | 变更 |
|------|------|
| `sharing/store.go` | 新增 SkillRegistryEntry、LobsterSkill 数据模型及 CRUD |
| `claw/service.go` | 新增 InstallSkill/UninstallSkill/ListInstalledSkills; StartInstance 加 HOME 隔离 |
| `server/app.go` | 新增 Skill 库浏览 + 龙虾 Skill 管理 + 管理员 Skill 管理 API 路由和 Handler |
| `docs/skill-injection-design.md` | 本设计文档 |