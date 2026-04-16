package claw

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pinhaoclaw/pinhaoclaw/claw/backend"
	"github.com/pinhaoclaw/pinhaoclaw/sharing"
)

// NodeService 节点管理服务（生产环境使用 SSH）
type NodeService struct {
	store *sharing.Store
}

func NewNodeService(store *sharing.Store) *NodeService {
	return &NodeService{store: store}
}

// SSHClientFor 为指定节点创建 SSH 客户端
func (s *NodeService) SSHClientFor(node *sharing.Node) *backend.SSHClient {
	opts := []backend.SSHOpts{
		backend.WithPort(node.SSHPort),
		backend.WithUser(node.SSHUser),
		backend.WithKeyPath(node.SSHKeyPath),
		backend.WithPassword(node.SSHPassword),
	}
	return backend.NewSSHClient(node.Host, opts...)
}

// runnerFor 返回节点的 CommandRunner（生产环境用 SSH）
func (s *NodeService) runnerFor(node *sharing.Node) backend.CommandRunner {
	if isLocalNode(node) {
		return backend.NewLocalRunner(s.resolveLocalPicoclawPath(node))
	}
	return s.SSHClientFor(node)
}

// ── 以下方法委托给 NodeServiceWithRunner ──

func (s *NodeService) TestConnection(ctx context.Context, node *sharing.Node) error {
	if isLocalNode(node) {
		return s.testLocalConnection(ctx, node)
	}
	return s.SSHClientFor(node).CheckConnection(ctx)
}

func (s *NodeService) DetectEnvironment(ctx context.Context, node *sharing.Node) (map[string]string, error) {
	runner := s.runnerFor(node)
	script := `set -e
echo "OS=$(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d'"' -f2 || echo Unknown)"
echo "ARCH=$(uname -m)"
echo "CPU=$(nproc)"
echo "MEMORY=$(free -h 2>/dev/null | awk '/Mem:/ {print $2}' || echo N/A)"
echo "DISK=$(df -h / 2>/dev/null | awk 'NR==2 {print $4}' || echo N/A)"
echo "PICOCLAW=$(which picoclaw 2>/dev/null && picoclaw version 2>&1 | tail -1 || echo not_found)"
`
	result, err := runner.Run(ctx, script)
	if err != nil {
		return nil, err
	}
	info := make(map[string]string)
	for _, line := range strings.Split(result.Stdout, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) == 2 {
			info[parts[0]] = parts[1]
		}
	}
	return info, nil
}

func (s *NodeService) Deploy(ctx context.Context, node *sharing.Node, eventCh chan<- SSEEvent) {
	defer close(eventCh)

	runner := s.runnerFor(node)

	send := func(stage, msg string) {
		select {
		case eventCh <- NewProgress(stage, msg):
		case <-ctx.Done():
		}
	}

	if isLocalNode(node) {
		send("connect", fmt.Sprintf("检查本地节点目录 %s ...", node.RemoteHome))
		if err := s.testLocalConnection(ctx, node); err != nil {
			eventCh <- NewError("本地节点不可用: " + err.Error())
			return
		}
		send("connected", fmt.Sprintf("✓ 本地节点已就绪: %s", node.RemoteHome))
	} else {
		send("connect", fmt.Sprintf("正在连接 %s ...", node.Host))
		if err := s.SSHClientFor(node).CheckConnection(ctx); err != nil {
			eventCh <- NewError("SSH 连接失败: " + err.Error())
			return
		}
		send("connected", fmt.Sprintf("✓ 已连接 %s", node.Host))
	}

	// Step 2: 检查远端 picoclaw
	send("check", map[bool]string{true: "检查本地 picoclaw...", false: "检查远端 picoclaw..."}[isLocalNode(node)])
	checkResult, _ := runner.Run(ctx, "which picoclaw >/dev/null 2>&1 && echo EXISTS || echo MISSING")
	if strings.Contains(checkResult.Stdout, "EXISTS") {
		send("found", "✓ picoclaw 已安装")
	} else {
		eventCh <- NewError(map[bool]string{true: "本地未找到 picoclaw，请设置 PINHAOCLAW_PICOCLAW_BIN 或 node.picoclaw_path", false: "远端未安装 picoclaw，请手动安装后重试"}[isLocalNode(node)])
		return
	}

	// Step 3: 初始化目录
	remoteHome := node.RemoteHome
	if remoteHome == "" {
		if isLocalNode(node) {
			remoteHome = filepath.Join(os.TempDir(), "pinhaoclaw-local-node")
		} else {
			remoteHome = "/opt/pinhaoclaw"
		}
	}
	send("init", map[bool]string{true: "初始化本地目录...", false: "初始化远端目录..."}[isLocalNode(node)])
	_, err := runner.Run(ctx, fmt.Sprintf("mkdir -p %s/{instances,logs,ports}", shellEscape(remoteHome)))
	if err != nil {
		eventCh <- NewError("目录初始化失败: " + err.Error())
		return
	}
	send("init_ok", "✓ 目录结构已就绪")

	// Step 4: 验证
	send("verify", "验证 picoclaw 可用性...")
	verResult, _ := runner.Run(ctx, "picoclaw version 2>&1 | grep -o 'picoclaw.*' | head -1")
	ver := strings.TrimSpace(verResult.Stdout)
	if ver == "" {
		ver = "unknown"
	}
	send("verify_ok", fmt.Sprintf("✓ %s", ver))

	// 更新节点状态
	node.Status = "online"
	if isLocalNode(node) {
		node.PicoClawPath = s.resolveLocalPicoclawPath(node)
	} else {
		node.PicoClawPath = "/usr/local/bin/picoclaw"
	}
	if node.RemoteHome == "" {
		node.RemoteHome = remoteHome
	}
	s.store.SaveNode(node)

	eventCh <- SSEEvent{Event: "done", Data: map[string]any{
		"stage":   "deploy_complete",
		"message": "节点部署完成，可以开始创建龙虾了！",
		"version": ver,
	}}
}

func (s *NodeService) AllocatePort(ctx context.Context, node *sharing.Node) (int, error) {
	r := &NodeServiceWithRunner{runner: s.runnerFor(node), store: s.store}
	return r.AllocatePort(ctx, node)
}

func (s *NodeService) CreateInstance(ctx context.Context, node *sharing.Node, lobsterID string, port int) error {
	r := &NodeServiceWithRunner{runner: s.runnerFor(node), store: s.store}
	return r.CreateInstance(ctx, node, lobsterID, port)
}

func (s *NodeService) StartInstance(ctx context.Context, node *sharing.Node, lobsterID string, port int) error {
	r := &NodeServiceWithRunner{runner: s.runnerFor(node), store: s.store}
	return r.StartInstance(ctx, node, lobsterID, port)
}

func (s *NodeService) BindWeixin(ctx context.Context, node *sharing.Node, lobsterID string) (<-chan string, <-chan error) {
	r := &NodeServiceWithRunner{runner: s.runnerFor(node), store: s.store}
	return r.BindWeixin(ctx, node, lobsterID)
}

func (s *NodeService) RestartInstance(ctx context.Context, node *sharing.Node, lobster *sharing.Lobster) error {
	r := &NodeServiceWithRunner{runner: s.runnerFor(node), store: s.store}
	return r.RestartInstance(ctx, node, lobster)
}

// RestartGateway 重启远端所有 gateway（仅用于部署后初始化）
func (s *NodeService) RestartGateway(ctx context.Context, node *sharing.Node) error {
	if isLocalNode(node) {
		return nil
	}
	ssh := s.SSHClientFor(node)
	_, _ = ssh.Run(ctx, "pkill -f 'picoclaw gateway' 2>/dev/null; sleep 1")
	_, err := ssh.Run(ctx, "nohup picoclaw gateway > /root/.picoclaw/logs/gateway.log 2>&1 &")
	if err != nil {
		return err
	}
	for i := 0; i < 15; i++ {
		time.Sleep(1 * time.Second)
		r, _ := ssh.Run(ctx, "curl -sf http://127.0.0.1:18790/health 2>/dev/null || echo fail")
		if strings.Contains(r.Stdout, "ok") {
			return nil
		}
	}
	return fmt.Errorf("gateway health check timeout")
}

func (s *NodeService) StopInstance(ctx context.Context, node *sharing.Node, lobsterID string) error {
	r := &NodeServiceWithRunner{runner: s.runnerFor(node), store: s.store}
	return r.StopInstance(ctx, node, lobsterID)
}

func (s *NodeService) RemoveInstance(ctx context.Context, node *sharing.Node, lobsterID string, port int) error {
	r := &NodeServiceWithRunner{runner: s.runnerFor(node), store: s.store}
	return r.RemoveInstance(ctx, node, lobsterID, port)
}

func (s *NodeService) ListInstances(ctx context.Context, node *sharing.Node) ([]map[string]string, error) {
	runner := s.runnerFor(node)
	shellInstancesDir := shellEscape(node.RemoteHome + "/instances")
	script := fmt.Sprintf(`for d in %s/*/; do
  [ -d "$d" ] || continue
  id="$(basename $d)"
  echo "ID=$id"
done`, shellInstancesDir)
	result, err := runner.Run(ctx, script)
	if err != nil {
		return nil, err
	}
	var instances []map[string]string
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			instances = append(instances, map[string]string{"id": strings.TrimPrefix(line, "ID=")})
		}
	}
	return instances, nil
}

func isLocalNode(node *sharing.Node) bool {
	return node != nil && strings.EqualFold(strings.TrimSpace(node.Type), "local")
}

func (s *NodeService) testLocalConnection(ctx context.Context, node *sharing.Node) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}
	if strings.TrimSpace(node.RemoteHome) == "" {
		return fmt.Errorf("local node remote_home is required")
	}
	if err := os.MkdirAll(node.RemoteHome, 0o755); err != nil {
		return fmt.Errorf("create local node home: %w", err)
	}
	picoclawBin := s.resolveLocalPicoclawPath(node)
	if picoclawBin == "" {
		return fmt.Errorf("picoclaw binary not found")
	}
	node.PicoClawPath = picoclawBin
	runner := backend.NewLocalRunner(picoclawBin)
	if _, err := runner.Run(ctx, fmt.Sprintf("mkdir -p %s && picoclaw version >/dev/null", shellEscape(node.RemoteHome))); err != nil {
		return err
	}
	return nil
}

func (s *NodeService) resolveLocalPicoclawPath(node *sharing.Node) string {
	candidates := []string{}
	if node != nil {
		candidates = append(candidates, strings.TrimSpace(node.PicoClawPath))
	}
	candidates = append(candidates, strings.TrimSpace(os.Getenv("PINHAOCLAW_PICOCLAW_BIN")))
	if path, err := exec.LookPath("picoclaw"); err == nil {
		candidates = append(candidates, path)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "..", "picoclaw", "build", "picoclaw"),
			filepath.Join(wd, "..", "picoclaw", "build", "picoclaw-linux-amd64"),
		)
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "picoclaw", "build", "picoclaw"),
			filepath.Join(exeDir, "..", "picoclaw", "build", "picoclaw-linux-amd64"),
		)
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// ── NodeServiceWithRunner: 可注入 mock runner 的实现 ──

var safeIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// shellEscape wraps a string in single quotes for safe shell interpolation.
// Any single quotes within the string are escaped as '\”.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// validateSafeID checks that an ID contains only safe characters (alphanumeric, underscore, hyphen).
func validateSafeID(id string) error {
	if !safeIDRe.MatchString(id) {
		return fmt.Errorf("invalid ID: %q (only alphanumeric, underscore, hyphen allowed)", id)
	}
	return nil
}

type NodeServiceWithRunner struct {
	runner backend.CommandRunner
	store  *sharing.Store
}

func instanceDir(node *sharing.Node, lobsterID string) string {
	return fmt.Sprintf("%s/instances/%s", node.RemoteHome, lobsterID)
}

// shellInstanceDir returns the unescaped instance directory path.
// Use shellEscape() when interpolating into shell commands.
func shellInstanceDir(node *sharing.Node, lobsterID string) string {
	return fmt.Sprintf("%s/instances/%s", node.RemoteHome, lobsterID)
}

// CreateInstance 在远端创建龙虾实例目录和启动脚本
func (s *NodeServiceWithRunner) CreateInstance(ctx context.Context, node *sharing.Node, lobsterID string, port int) error {
	if err := validateSafeID(lobsterID); err != nil {
		return err
	}
	if port < 8100 || port > 8300 {
		return fmt.Errorf("port %d out of range [8100, 8300]", port)
	}
	instDir := shellInstanceDir(node, lobsterID)
	shellDir := shellEscape(instDir)

	// 创建目录结构
	mkdirScript := fmt.Sprintf("mkdir -p %s/{workspace,logs,skills}", shellDir)
	if _, err := s.runner.Run(ctx, mkdirScript); err != nil {
		return err
	}

	// 生成 start.sh 启动脚本（heredoc 内使用引号路径）
	shellWorkspace := shellEscape(instDir + "/workspace")
	shellStartSh := shellEscape(instDir + "/start.sh")
	startSh := fmt.Sprintf(`#!/bin/bash
cd %s
picoclaw gateway --home . --port %d >> ../logs/gateway.log 2>&1 &
echo $! > ../picoclaw.pid`, shellWorkspace, port)
	writeScript := fmt.Sprintf("cat > %s << 'STARTSH'\n%s\nSTARTSH\nchmod +x %s", shellStartSh, startSh, shellStartSh)
	_, err := s.runner.Run(ctx, writeScript)
	return err
}

// StartInstance 启动远端龙虾实例
// 设置 HOME 环境变量指向龙虾实例目录，确保 picoclaw 的全局 skills 路径
// (~/.picoclaw/skills) 被限定在龙虾独立空间内，实现不同龙虾之间的 Skill 隔离。
func (s *NodeServiceWithRunner) StartInstance(ctx context.Context, node *sharing.Node, lobsterID string, port int) error {
	if err := validateSafeID(lobsterID); err != nil {
		return err
	}
	if port < 8100 || port > 8300 {
		return fmt.Errorf("port %d out of range [8100, 8300]", port)
	}
	instDir := shellInstanceDir(node, lobsterID)
	shellWorkspace := shellEscape(instDir + "/workspace")
	shellInstDir := shellEscape(instDir)
	script := fmt.Sprintf(
		"cd %s && HOME=%s nohup picoclaw gateway --home . --port %d >> ../logs/gateway.log 2>&1 & echo $! > ../picoclaw.pid",
		shellWorkspace, shellInstDir, port,
	)
	_, err := s.runner.Run(ctx, script)
	return err
}

// StopInstance 停止远端龙虾实例
func (s *NodeServiceWithRunner) StopInstance(ctx context.Context, node *sharing.Node, lobsterID string) error {
	if err := validateSafeID(lobsterID); err != nil {
		return err
	}
	pidFile := shellEscape(fmt.Sprintf("%s/instances/%s/picoclaw.pid", node.RemoteHome, lobsterID))
	script := fmt.Sprintf(`if [ -f %s ]; then
  PID=$(cat %s)
  kill $PID 2>/dev/null; sleep 1; kill -9 $PID 2>/dev/null || true
  rm -f %s
fi`, pidFile, pidFile, pidFile)
	_, err := s.runner.Run(ctx, script)
	return err
}

// BindWeixin 通过 SSH 流式执行 picoclaw auth weixin（指定 --home）
func (s *NodeServiceWithRunner) BindWeixin(ctx context.Context, node *sharing.Node, lobsterID string) (<-chan string, <-chan error) {
	if err := validateSafeID(lobsterID); err != nil {
		errCh := make(chan error, 1)
		errCh <- err
		return nil, errCh
	}
	instDir := shellInstanceDir(node, lobsterID)
	shellWorkspace := shellEscape(instDir + "/workspace")
	cmd := fmt.Sprintf("picoclaw auth weixin --home %s 2>&1", shellWorkspace)
	return s.runner.StreamRun(ctx, cmd)
}

// RestartInstance 重启指定龙虾实例（不影响其他实例）
func (s *NodeServiceWithRunner) RestartInstance(ctx context.Context, node *sharing.Node, lobster *sharing.Lobster) error {
	if err := s.StopInstance(ctx, node, lobster.ID); err != nil {
		// 停止失败不阻止重启，可能是进程已退出
	}
	return s.StartInstance(ctx, node, lobster.ID, lobster.Port)
}

// AllocatePort 使用 mkdir 原子操作分配端口（消除竞态）
func (s *NodeServiceWithRunner) AllocatePort(ctx context.Context, node *sharing.Node) (int, error) {
	shellPortsDir := shellEscape(node.RemoteHome + "/ports")
	script := fmt.Sprintf(`mkdir -p %s
for port in $(seq 8100 8300); do
  if mkdir %s/$port 2>/dev/null; then
    echo $port
    break
  fi
done`, shellPortsDir, shellPortsDir)

	result, err := s.runner.Run(ctx, script)
	if err != nil {
		return 0, err
	}
	var port int
	fmt.Sscanf(strings.TrimSpace(result.Stdout), "%d", &port)
	if port <= 0 {
		return 0, fmt.Errorf("no available port")
	}
	return port, nil
}

// InstallSkill 将 Skill 注入到指定龙虾的 skills 目录
func (s *NodeService) InstallSkill(ctx context.Context, node *sharing.Node, lobsterID string, skill *sharing.SkillRegistryEntry) error {
	if err := validateSafeID(lobsterID); err != nil {
		return err
	}
	runner := s.runnerFor(node)
	instDir := shellInstanceDir(node, lobsterID)
	skillDir := fmt.Sprintf("%s/skills/%s", instDir, skill.Slug)
	shellSkillDir := shellEscape(skillDir)

	switch skill.Source.Type {
	case "github":
		script := fmt.Sprintf(`set -e
mkdir -p %s
if command -v git >/dev/null 2>&1; then
  git clone --depth 1 https://github.com/%s %s 2>/dev/null || true
  rm -rf %s/.git
else
  curl -sfL "https://raw.githubusercontent.com/%s/main/SKILL.md" -o %s/SKILL.md || true
fi
if [ ! -f %s/SKILL.md ]; then
  echo "ERROR: SKILL.md not found" >&2
  exit 1
fi`, shellSkillDir, shellEscape(skill.Source.Repo), shellSkillDir, shellSkillDir,
			shellEscape(skill.Source.Repo), shellSkillDir, shellSkillDir)
		_, err := runner.Run(ctx, script)
		return err

	case "builtin":
		if skill.Source.LocalDir == "" {
			return fmt.Errorf("builtin skill %q missing local_dir", skill.Slug)
		}
		script := fmt.Sprintf(`set -e
mkdir -p %s
cp -r %s/* %s/ 2>/dev/null || true
if [ ! -f %s/SKILL.md ]; then
  echo "ERROR: SKILL.md not found" >&2
  exit 1
fi`, shellSkillDir, shellEscape(skill.Source.LocalDir), shellSkillDir, shellSkillDir)
		_, err := runner.Run(ctx, script)
		return err

	case "clawhub":
		shellWorkspace := shellEscape(instDir + "/workspace")
		script := fmt.Sprintf(`set -e
cd %s
picoclaw skill install --slug %s --target %s`, shellWorkspace, shellEscape(skill.Source.ClawHub), shellSkillDir)
		_, err := runner.Run(ctx, script)
		return err

	default:
		return fmt.Errorf("unsupported skill source type: %q", skill.Source.Type)
	}
}

// UninstallSkill 从龙虾中卸载 Skill
func (s *NodeService) UninstallSkill(ctx context.Context, node *sharing.Node, lobsterID string, skillSlug string) error {
	if err := validateSafeID(lobsterID); err != nil {
		return err
	}
	if err := validateSafeID(skillSlug); err != nil {
		return err
	}
	runner := s.runnerFor(node)
	instDir := shellInstanceDir(node, lobsterID)
	script := fmt.Sprintf("rm -rf %s/skills/%s", shellEscape(instDir), shellEscape(skillSlug))
	_, err := runner.Run(ctx, script)
	return err
}

// ListInstalledSkills 列出龙虾已安装的 Skill slug 列表
func (s *NodeService) ListInstalledSkills(ctx context.Context, node *sharing.Node, lobsterID string) ([]string, error) {
	if err := validateSafeID(lobsterID); err != nil {
		return nil, err
	}
	runner := s.runnerFor(node)
	instDir := shellInstanceDir(node, lobsterID)
	script := fmt.Sprintf(`ls -1 %s/skills/ 2>/dev/null || true`, shellEscape(instDir))
	result, err := runner.Run(ctx, script)
	if err != nil {
		return nil, err
	}
	var skills []string
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			skills = append(skills, line)
		}
	}
	return skills, nil
}

// RemoveInstance 删除远端龙虾实例（先停止再删除，释放端口）
func (s *NodeServiceWithRunner) RemoveInstance(ctx context.Context, node *sharing.Node, lobsterID string, port int) error {
	if err := validateSafeID(lobsterID); err != nil {
		return err
	}
	_ = s.StopInstance(ctx, node, lobsterID)
	instDir := shellInstanceDir(node, lobsterID)
	shellDir := shellEscape(instDir)
	// 释放端口
	if port > 0 {
		shellPortsDir := shellEscape(node.RemoteHome + "/ports")
		_, _ = s.runner.Run(ctx, fmt.Sprintf("rmdir %s/%d 2>/dev/null", shellPortsDir, port))
	}
	_, err := s.runner.Run(ctx, fmt.Sprintf("rm -rf %s", shellDir))
	return err
}

// ── SSE Event helpers ────────────────────────────────

type SSEEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

func (e SSEEvent) ToSSEFormat() string {
	dataBytes, _ := json.Marshal(e.Data)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", e.Event, string(dataBytes))
}

func NewProgress(stage, message string) SSEEvent {
	return SSEEvent{Event: "progress", Data: map[string]string{"stage": stage, "message": message}}
}

func NewError(message string) SSEEvent {
	return SSEEvent{Event: "error", Data: map[string]string{"stage": "error", "message": message}}
}
