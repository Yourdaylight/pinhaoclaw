package claw

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/garden/pinhaoclaw/claw/backend"
	"github.com/garden/pinhaoclaw/sharing"
)

// NodeService 节点管理服务
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

// TestConnection 测试节点 SSH 连通性
func (s *NodeService) TestConnection(ctx context.Context, node *sharing.Node) error {
	ssh := s.SSHClientFor(node)
	return ssh.CheckConnection(ctx)
}

// DetectEnvironment 检测远端环境信息
func (s *NodeService) DetectEnvironment(ctx context.Context, node *sharing.Node) (map[string]string, error) {
	ssh := s.SSHClientFor(node)
	script := `set -e
echo "OS=$(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d'"' -f2 || echo Unknown)"
echo "ARCH=$(uname -m)"
echo "CPU=$(nproc)"
echo "MEMORY=$(free -h 2>/dev/null | awk '/Mem:/ {print $2}' || echo N/A)"
echo "DISK=$(df -h / 2>/dev/null | awk 'NR==2 {print $4}' || echo N/A)"
echo "PICOCLAW=$(which picoclaw 2>/dev/null && picoclaw version 2>&1 | tail -1 || echo not_found)"
`
	result, err := ssh.Run(ctx, script)
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

// Deploy 一键部署 picoclaw 到节点（SSE 流式输出）
func (s *NodeService) Deploy(ctx context.Context, node *sharing.Node, eventCh chan<- SSEEvent) {
	defer close(eventCh)

	ssh := s.SSHClientFor(node)

	send := func(stage, msg string) {
		select {
		case eventCh <- NewProgress(stage, msg):
		case <-ctx.Done():
		}
	}

	// Step 1: SSH 连接
	send("connect", fmt.Sprintf("正在连接 %s ...", node.Host))
	if err := ssh.CheckConnection(ctx); err != nil {
		eventCh <- NewError("SSH 连接失败: " + err.Error())
		return
	}
	send("connected", fmt.Sprintf("✓ 已连接 %s", node.Host))

	// Step 2: 检查远端 picoclaw
	send("check", "检查远端 picoclaw...")
	checkResult, _ := ssh.Run(ctx, "test -x /usr/local/bin/picoclaw && echo EXISTS || echo MISSING")
	if strings.Contains(checkResult.Stdout, "EXISTS") {
		send("found", "✓ picoclaw 已安装")
	} else {
		eventCh <- NewError("远端未安装 picoclaw，请手动安装后重试")
		return
	}

	// Step 3: 初始化目录
	remoteHome := node.RemoteHome
	if remoteHome == "" {
		remoteHome = "/opt/shareclaw"
	}
	send("init", "初始化远端目录...")
	_, err := ssh.Run(ctx, fmt.Sprintf("mkdir -p %s/{instances,logs}", remoteHome))
	if err != nil {
		eventCh <- NewError("目录初始化失败: " + err.Error())
		return
	}
	send("init_ok", "✓ 目录结构已就绪")

	// Step 4: 验证
	send("verify", "验证 picoclaw 可用性...")
	verResult, _ := ssh.Run(ctx, "picoclaw version 2>&1 | grep -o 'picoclaw.*' | head -1")
	ver := strings.TrimSpace(verResult.Stdout)
	if ver == "" {
		ver = "unknown"
	}
	send("verify_ok", fmt.Sprintf("✓ %s", ver))

	// 更新节点状态
	node.Status = "online"
	node.PicoClawPath = "/usr/local/bin/picoclaw"
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

// AllocatePort 在远端分配端口
func (s *NodeService) AllocatePort(ctx context.Context, node *sharing.Node) (int, error) {
	ssh := s.SSHClientFor(node)
	script := fmt.Sprintf(`
ALLOC_FILE=%s/port_alloc.json
if [ -f "$ALLOC_FILE" ]; then
  NEXT=$(grep -o '"next_port":[[:space:]]*[0-9]*' "$ALLOC_FILE" | grep -o '[0-9]*')
else
  NEXT=8100
fi
PORT=${NEXT:-8100}
NEXT=$((PORT + 1))
echo "{\"next_port\":$NEXT}" > "$ALLOC_FILE"
echo $PORT`, node.RemoteHome)

	result, err := ssh.Run(ctx, script)
	if err != nil {
		return 0, err
	}
	var port int
	fmt.Sscanf(strings.TrimSpace(result.Stdout), "%d", &port)
	if port <= 0 {
		port = 8100
	}
	return port, nil
}

// CreateInstance 在远端创建龙虾实例目录
func (s *NodeService) CreateInstance(ctx context.Context, node *sharing.Node, lobsterID string, port int) error {
	ssh := s.SSHClientFor(node)
	instDir := fmt.Sprintf("%s/instances/%s", node.RemoteHome, lobsterID)
	script := fmt.Sprintf("mkdir -p %s/{workspace,logs,skills}", instDir)
	_, err := ssh.Run(ctx, script)
	return err
}

// BindWeixin 通过 SSH 流式执行 picoclaw auth weixin，返回输出行
func (s *NodeService) BindWeixin(ctx context.Context, node *sharing.Node) (<-chan string, <-chan error) {
	ssh := s.SSHClientFor(node)
	return ssh.StreamRun(ctx, "picoclaw auth weixin 2>&1")
}

// RestartGateway 重启远端 gateway
func (s *NodeService) RestartGateway(ctx context.Context, node *sharing.Node) error {
	ssh := s.SSHClientFor(node)
	_, _ = ssh.Run(ctx, "pkill -f 'picoclaw gateway' 2>/dev/null; sleep 1")
	_, err := ssh.Run(ctx, "nohup picoclaw gateway > /root/.picoclaw/logs/gateway.log 2>&1 &")
	if err != nil {
		return err
	}
	// 等待健康
	for i := 0; i < 15; i++ {
		time.Sleep(1 * time.Second)
		r, _ := ssh.Run(ctx, "curl -sf http://127.0.0.1:18790/health 2>/dev/null || echo fail")
		if strings.Contains(r.Stdout, "ok") {
			return nil
		}
	}
	return fmt.Errorf("gateway health check timeout")
}

// StopInstance 停止远端龙虾实例
func (s *NodeService) StopInstance(ctx context.Context, node *sharing.Node, lobsterID string) error {
	ssh := s.SSHClientFor(node)
	pidFile := fmt.Sprintf("%s/instances/%s/picoclaw.pid", node.RemoteHome, lobsterID)
	script := fmt.Sprintf(`if [ -f %s ]; then
  kill $(cat %s) 2>/dev/null; sleep 1; kill -9 $(cat %s) 2>/dev/null || true; rm -f %s
fi`, pidFile, pidFile, pidFile, pidFile)
	_, err := ssh.Run(ctx, script)
	return err
}

// RemoveInstance 删除远端龙虾实例
func (s *NodeService) RemoveInstance(ctx context.Context, node *sharing.Node, lobsterID string) error {
	_ = s.StopInstance(ctx, node, lobsterID)
	ssh := s.SSHClientFor(node)
	instDir := fmt.Sprintf("%s/instances/%s", node.RemoteHome, lobsterID)
	_, err := ssh.Run(ctx, fmt.Sprintf("rm -rf %s", instDir))
	return err
}

// ListInstances 列出远端实例
func (s *NodeService) ListInstances(ctx context.Context, node *sharing.Node) ([]map[string]string, error) {
	ssh := s.SSHClientFor(node)
	script := fmt.Sprintf(`for d in %s/instances/*/; do
  [ -d "$d" ] || continue
  id="$(basename $d)"
  echo "ID=$id"
done`, node.RemoteHome)
	result, err := ssh.Run(ctx, script)
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
