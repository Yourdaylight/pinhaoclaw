package backend

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SSHClient 封装 SSH 连接，用于远程服务器操作
type SSHClient struct {
	Host       string // IP 或域名
	Port       int    // SSH 端口（默认 22）
	User       string // 用户名（默认 root）
	KeyPath    string // 私钥路径
	Password   string // 密码（优先级低于 KeyPath）
	Timeout    time.Duration
}

// SSHPayload 远程命令执行结果
type SSHPayload struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// NewSSHClient 创建 SSH 客户端
func NewSSHClient(host string, opts ...SSHOpts) *SSHClient {
	c := &SSHClient{
		Host:    host,
		Port:    22,
		User:    "root",
		Timeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SSHOpts SSH 配置选项
type SSHOpts func(*SSHClient)

func WithPort(port int) SSHOpts        { return func(c *SSHClient) { if port > 0 { c.Port = port } } }
func WithUser(user string) SSHOpts      { return func(c *SSHClient) { if user != "" { c.User = user } } }
func WithKeyPath(path string) SSHOpts   { return func(c *SSHClient) { if path != "" { c.KeyPath = path } } }
func WithPassword(pw string) SSHOpts    { return func(c *SSHClient) { if pw != "" { c.Password = pw } } }
func WithTimeout(d time.Duration) SSHOpts { return func(c *SSHClient) { if d > 0 { c.Timeout = d } } }

// Run 在远程主机执行命令并返回结果
func (c *SSHClient) Run(ctx context.Context, cmd string) (*SSHPayload, error) {
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		fmt.Sprintf("-p%d", c.Port),
	}
	if c.KeyPath != "" {
		args = append(args, "-i", c.KeyPath)
	} else if c.Password != "" {
		// sshpass 方式（需要系统安装 sshpass）
		return c.runWithSSHPass(ctx, cmd)
	}
	args = append(args, fmt.Sprintf("%s@%s", c.User, c.Host), cmd)

	runCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(runCtx, "ssh", args...)
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()

	result := &SSHPayload{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: 0,
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, fmt.Errorf("ssh %s@%s: %s (exit=%d)", c.User, c.Host, result.Stderr, result.ExitCode)
	}
	return result, nil
}

// runWithSSHPass 使用 sshpass 执行带密码的 SSH 命令
func (c *SSHClient) runWithSSHPass(ctx context.Context, cmd string) (*SSHPayload, error) {
	args := []string{"-p", c.Password, "ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", fmt.Sprintf("Port=%d", c.Port),
		fmt.Sprintf("%s@%s", c.User, c.Host), cmd,
	}

	runCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(runCtx, "sshpass", args...)
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	result := &SSHPayload{
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, fmt.Errorf("sshpass: %s", result.Stderr)
	}
	return result, nil
}

// SCPUpload 通过 SCP 上传本地文件到远程主机
func (c *SSHClient) SCPUpload(ctx context.Context, localPath, remotePath string) error {
	scpArgs := []string{
		"-o", "StrictHostKeyChecking=no",
	}
	if c.KeyPath != "" {
		scpArgs = append(scpArgs, "-o", "BatchMode=yes", "-i", c.KeyPath)
	}
	scpArgs = append(scpArgs,
		fmt.Sprintf("-P%d", c.Port),
		localPath,
		fmt.Sprintf("%s@%s:%s", c.User, c.Host, remotePath),
	)

	runCtx, cancel := context.WithTimeout(ctx, c.Timeout*3)
	defer cancel()

	var cmd *exec.Cmd
	if c.Password != "" && c.KeyPath == "" {
		// sshpass 包裹 scp
		fullArgs := append([]string{"-p", c.Password, "scp"}, scpArgs...)
		cmd = exec.CommandContext(runCtx, "sshpass", fullArgs...)
	} else {
		cmd = exec.CommandContext(runCtx, "scp", scpArgs...)
	}
	cmd.Stdout = nil
	cmd.Stderr = nil

	return cmd.Run()
}

// StreamRun 流式执行远程命令（适合长时间运行的任务）
// 返回 stdout 行的 channel 和 error channel
func (c *SSHClient) StreamRun(ctx context.Context, cmd string) (<-chan string, <-chan error) {
	outCh := make(chan string, 50)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)

		sshArgs := []string{
			"-o", "StrictHostKeyChecking=no",
			"-o", fmt.Sprintf("Port=%d", c.Port),
			"-tt", // 强制分配 PTY，让远端命令输出二维码等终端内容
		}
		if c.KeyPath != "" {
			sshArgs = append(sshArgs, "-o", "BatchMode=yes", "-i", c.KeyPath)
		}
		sshArgs = append(sshArgs, fmt.Sprintf("%s@%s", c.User, c.Host), cmd)

		// 微信扫码登录需要 5 分钟超时
		streamTimeout := 6 * time.Minute
		runCtx, cancel := context.WithTimeout(ctx, streamTimeout)
		defer cancel()

		var execCmd *exec.Cmd
		if c.Password != "" && c.KeyPath == "" {
			fullArgs := append([]string{"-p", c.Password, "ssh"}, sshArgs...)
			execCmd = exec.CommandContext(runCtx, "sshpass", fullArgs...)
		} else {
			execCmd = exec.CommandContext(runCtx, "ssh", sshArgs...)
		}

		stdoutPipe, err := execCmd.StdoutPipe()
		if err != nil {
			errCh <- fmt.Errorf("pipe stdout: %w", err)
			return
		}

		stderrPipe, err := execCmd.StderrPipe()
		if err != nil {
			errCh <- fmt.Errorf("pipe stderr: %w", err)
			return
		}

		if err := execCmd.Start(); err != nil {
			errCh <- fmt.Errorf("start command: %w", err)
			return
		}

		// 异步读取 stderr 到错误通道
		go func() {
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				// stderr 内容作为日志记录，不中断
				line := scanner.Text()
				if strings.Contains(strings.ToLower(line), "error") ||
					strings.Contains(strings.ToLower(line), "fatal") {
					_ = line // 可选：发送到某个日志通道
				}
			}
		}()

		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			select {
			case outCh <- scanner.Text():
			case <-ctx.Done():
				_ = execCmd.Process.Kill()
				errCh <- ctx.Err()
				return
			}
		}

		if err := execCmd.Wait(); err != nil {
			select {
			case errCh <- fmt.Errorf("command exited: %w", err):
			default:
			}
			return
		}
		close(errCh)
	}()

	return outCh, errCh
}

// CheckConnection 测试 SSH 连通性
func (c *SSHClient) CheckConnection(ctx context.Context) error {
	result, err := c.Run(ctx, "echo ok")
	if err != nil {
		return err
	}
	if result.Stdout != "ok" {
		return fmt.Errorf("unexpected output: %s", result.Stdout)
	}
	return nil
}

// String 返回连接描述
func (c *SSHClient) String() string {
	return fmt.Sprintf("ssh://%s@%s:%d", c.User, c.Host, c.Port)
}
