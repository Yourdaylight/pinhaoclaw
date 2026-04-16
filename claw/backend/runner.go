package backend

import "context"

// CommandRunner 抽象远程命令执行接口，便于测试时 mock
type CommandRunner interface {
	Run(ctx context.Context, cmd string) (*SSHPayload, error)
	StreamRun(ctx context.Context, cmd string) (<-chan string, <-chan error)
}

// 编译时验证 SSHClient 满足 CommandRunner 接口
var _ CommandRunner = (*SSHClient)(nil)
