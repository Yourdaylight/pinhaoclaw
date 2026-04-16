package claw

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/pinhaoclaw/pinhaoclaw/claw/backend"
	"github.com/pinhaoclaw/pinhaoclaw/sharing"
)

// mockRunner 记录所有调用参数，用于验证命令正确性
type mockRunner struct {
	mu       sync.Mutex
	commands []string
	// 按命令前缀返回预设结果
	runResults map[string]*backend.SSHPayload
	runErrors  map[string]error
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		runResults: make(map[string]*backend.SSHPayload),
		runErrors:  make(map[string]error),
	}
}

func (m *mockRunner) Run(ctx context.Context, cmd string) (*backend.SSHPayload, error) {
	m.mu.Lock()
	m.commands = append(m.commands, cmd)
	m.mu.Unlock()

	for prefix, result := range m.runResults {
		if strings.Contains(cmd, prefix) {
			return result, m.runErrors[prefix]
		}
	}
	return &backend.SSHPayload{Stdout: "ok"}, nil
}

func (m *mockRunner) StreamRun(ctx context.Context, cmd string) (<-chan string, <-chan error) {
	m.mu.Lock()
	m.commands = append(m.commands, cmd)
	m.mu.Unlock()

	outCh := make(chan string, 10)
	errCh := make(chan error, 1)
	close(outCh)
	close(errCh)
	return outCh, errCh
}

func (m *mockRunner) getCommands() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.commands))
	copy(result, m.commands)
	return result
}

func (m *mockRunner) lastCommand() string {
	cmds := m.getCommands()
	if len(cmds) == 0 {
		return ""
	}
	return cmds[len(cmds)-1]
}

func (m *mockRunner) containsCommand(substr string) bool {
	for _, cmd := range m.getCommands() {
		if strings.Contains(cmd, substr) {
			return true
		}
	}
	return false
}

// ── 测试辅助 ──

func testNode() *sharing.Node {
	return &sharing.Node{
		ID:          "node_test01",
		Name:        "测试节点",
		Host:        "1.2.3.4",
		SSHPort:     22,
		SSHUser:     "root",
		SSHPassword: "testpass",
		RemoteHome:  "/opt/pinhaoclaw",
	}
}

func testLobster() *sharing.Lobster {
	return &sharing.Lobster{
		ID:     "lobster_abc12345",
		UserID: "user_test01",
		Name:   "测试龙虾",
		NodeID: "node_test01",
		Port:   8101,
	}
}

// ── 测试用例 ──

func TestCreateInstance_GeneratesStartScript(t *testing.T) {
	runner := newMockRunner()
	svc := &NodeServiceWithRunner{runner: runner, store: nil}

	err := svc.CreateInstance(context.Background(), testNode(), "lobster_abc12345", 8101)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	// 应该包含创建目录和写入 start.sh 的命令
	cmds := runner.getCommands()
	if len(cmds) == 0 {
		t.Fatal("no commands were executed")
	}

	hasMkdir := false
	hasStartSh := false
	for _, cmd := range cmds {
		if strings.Contains(cmd, "mkdir") && strings.Contains(cmd, "lobster_abc12345") {
			hasMkdir = true
		}
		if strings.Contains(cmd, "start.sh") || strings.Contains(cmd, "picoclaw gateway") {
			hasStartSh = true
		}
	}
	if !hasMkdir {
		t.Error("CreateInstance did not create instance directory")
	}
	if !hasStartSh {
		t.Error("CreateInstance did not generate start script")
	}
}

func TestStartInstance_UsesCorrectHomeAndPort(t *testing.T) {
	runner := newMockRunner()
	svc := &NodeServiceWithRunner{runner: runner, store: nil}

	err := svc.StartInstance(context.Background(), testNode(), "lobster_abc12345", 8101)
	if err != nil {
		t.Fatalf("StartInstance failed: %v", err)
	}

	cmd := runner.lastCommand()

	// 验证命令包含 --home 参数指向实例 workspace
	if !strings.Contains(cmd, "--home") {
		t.Error("StartInstance command missing --home flag")
	}
	if !strings.Contains(cmd, "/opt/pinhaoclaw/instances/lobster_abc12345/workspace") {
		t.Errorf("StartInstance --home path incorrect, got: %s", cmd)
	}
	// 验证命令包含 --port 参数
	if !strings.Contains(cmd, "--port") {
		t.Error("StartInstance command missing --port flag")
	}
	if !strings.Contains(cmd, "8101") {
		t.Error("StartInstance --port value incorrect")
	}
	// 验证写入 PID 文件
	if !strings.Contains(cmd, "picoclaw.pid") {
		t.Error("StartInstance command should write PID file")
	}
	// 验证是 nohup 后台启动
	if !strings.Contains(cmd, "nohup") {
		t.Error("StartInstance should use nohup for background process")
	}
}

func TestStopInstance_ReadsPidFile(t *testing.T) {
	runner := newMockRunner()
	runner.runResults["cat"] = &backend.SSHPayload{Stdout: "12345"}
	svc := &NodeServiceWithRunner{runner: runner, store: nil}

	err := svc.StopInstance(context.Background(), testNode(), "lobster_abc12345")
	if err != nil {
		t.Fatalf("StopInstance failed: %v", err)
	}

	cmd := runner.lastCommand()
	// 验证从实例目录读取 PID
	if !strings.Contains(cmd, "lobster_abc12345/picoclaw.pid") {
		t.Errorf("StopInstance should read PID from instance dir, got: %s", cmd)
	}
}

func TestBindWeixin_UsesHomeFlag(t *testing.T) {
	runner := newMockRunner()
	svc := &NodeServiceWithRunner{runner: runner, store: nil}

	node := testNode()
	lobsterID := "lobster_abc12345"

	_, _ = svc.BindWeixin(context.Background(), node, lobsterID)

	cmd := runner.lastCommand()
	// 验证命令包含 --home 指向实例 workspace
	if !strings.Contains(cmd, "--home") {
		t.Error("BindWeixin command missing --home flag")
	}
	if !strings.Contains(cmd, "/opt/pinhaoclaw/instances/lobster_abc12345/workspace") {
		t.Errorf("BindWeixin --home path incorrect, got: %s", cmd)
	}
	if !strings.Contains(cmd, "picoclaw auth weixin") {
		t.Error("BindWeixin should execute picoclaw auth weixin")
	}
}

func TestRestartInstance_PerInstance(t *testing.T) {
	runner := newMockRunner()
	svc := &NodeServiceWithRunner{runner: runner, store: nil}

	node := testNode()
	lobster := testLobster()

	err := svc.RestartInstance(context.Background(), node, lobster)
	if err != nil {
		t.Fatalf("RestartInstance failed: %v", err)
	}

	cmds := runner.getCommands()
	// 不应包含 pkill -f 'picoclaw gateway'（全局杀进程）
	for _, cmd := range cmds {
		if strings.Contains(cmd, "pkill") && strings.Contains(cmd, "picoclaw gateway") && !strings.Contains(cmd, "lobster_abc12345") {
			t.Errorf("RestartInstance should NOT use global pkill, got: %s", cmd)
		}
	}
	// 应该先停止再启动
	hasStop := false
	hasStart := false
	for _, cmd := range cmds {
		if strings.Contains(cmd, "picoclaw.pid") && (strings.Contains(cmd, "kill") || strings.Contains(cmd, "cat")) {
			hasStop = true
		}
		if strings.Contains(cmd, "picoclaw gateway") && strings.Contains(cmd, "--home") {
			hasStart = true
		}
	}
	if !hasStop {
		t.Error("RestartInstance should stop the instance first")
	}
	if !hasStart {
		t.Error("RestartInstance should start the instance with --home flag")
	}
}

func TestRemoveInstance_StopsFirst(t *testing.T) {
	runner := newMockRunner()
	svc := &NodeServiceWithRunner{runner: runner, store: nil}

	err := svc.RemoveInstance(context.Background(), testNode(), "lobster_abc12345", 8101)
	if err != nil {
		t.Fatalf("RemoveInstance failed: %v", err)
	}

	cmds := runner.getCommands()
	// 应先停止（读 PID + kill），再 rm -rf
	hasStop := false
	hasRemove := false
	for _, cmd := range cmds {
		if strings.Contains(cmd, "picoclaw.pid") {
			hasStop = true
		}
		if strings.Contains(cmd, "rm -rf") && strings.Contains(cmd, "lobster_abc12345") {
			hasRemove = true
		}
	}
	if !hasStop {
		t.Error("RemoveInstance should stop instance first")
	}
	if !hasRemove {
		t.Error("RemoveInstance should remove instance directory")
	}
}

func TestAllocatePort_UsesMkdir(t *testing.T) {
	runner := newMockRunner()
	// mkdir 成功时 echo 端口号
	runner.runResults["mkdir"] = &backend.SSHPayload{Stdout: "8105"}
	svc := &NodeServiceWithRunner{runner: runner, store: nil}

	port, err := svc.AllocatePort(context.Background(), testNode())
	if err != nil {
		t.Fatalf("AllocatePort failed: %v", err)
	}
	if port <= 0 {
		t.Errorf("expected positive port, got %d", port)
	}
	// 验证使用了 mkdir 原子操作
	if !runner.containsCommand("mkdir") {
		t.Error("AllocatePort should use mkdir for atomic allocation")
	}
	if !runner.containsCommand("ports") {
		t.Error("AllocatePort should use ports directory")
	}
}

func TestRemoveInstance_ReleasesPort(t *testing.T) {
	runner := newMockRunner()
	svc := &NodeServiceWithRunner{runner: runner, store: nil}

	err := svc.RemoveInstance(context.Background(), testNode(), "lobster_abc12345", 8101)
	if err != nil {
		t.Fatalf("RemoveInstance failed: %v", err)
	}

	// 验证释放端口
	if !runner.containsCommand("rmdir") && !runner.containsCommand("8101") {
		t.Error("RemoveInstance should release allocated port")
	}
}
