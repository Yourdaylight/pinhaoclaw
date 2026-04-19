package claw

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/pinhaoclaw/pinhaoclaw/claw/backend"
	"github.com/pinhaoclaw/pinhaoclaw/sharing"
)

// NodeService 节点管理服务（生产环境使用 SSH）
type NodeService struct {
	store *sharing.Store
}

type PicoclawPackageInfo struct {
	ConfiguredPath string `json:"configured_path"`
	ResolvedPath   string `json:"resolved_path"`
	Version        string `json:"version"`
	ManagedPath    string `json:"managed_path"`
	SupportsWeixin bool   `json:"supports_weixin_auth"`
}

func NewNodeService(store *sharing.Store) *NodeService {
	return &NodeService{store: store}
}

func (s *NodeService) managedPicoclawPath() string {
	return filepath.Join(s.store.Dir(), "bin", "picoclaw")
}

func (s *NodeService) probePicoclawVersion(binPath string) string {
	binPath = strings.TrimSpace(binPath)
	if binPath == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

func (s *NodeService) probePicoclawAuthSubcommand(binPath, subcommand string) bool {
	binPath = strings.TrimSpace(binPath)
	subcommand = strings.TrimSpace(subcommand)
	if binPath == "" || subcommand == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "auth", "--help")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return false
	}
	pattern := regexp.MustCompile(`(^|[[:space:]])` + regexp.QuoteMeta(subcommand) + `([[:space:]]|$)`)
	return pattern.Match(out)
}

func (s *NodeService) GetPicoclawPackageInfo() *PicoclawPackageInfo {
	settings := s.store.ReadSettings()
	configured := strings.TrimSpace(settings.PicoclawPackagePath)
	resolved := s.resolveLocalPicoclawPath(nil)
	return &PicoclawPackageInfo{
		ConfiguredPath: configured,
		ResolvedPath:   resolved,
		Version:        s.probePicoclawVersion(resolved),
		ManagedPath:    s.managedPicoclawPath(),
		SupportsWeixin: s.probePicoclawAuthSubcommand(resolved, "weixin"),
	}
}

func (s *NodeService) SetPicoclawPackagePath(path string) (*PicoclawPackageInfo, error) {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" {
		return nil, fmt.Errorf("path is empty")
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path must be a file")
	}
	if v := s.probePicoclawVersion(cleaned); v == "" {
		return nil, fmt.Errorf("invalid picoclaw binary or version probe failed")
	}
	settings := s.store.ReadSettings()
	settings.PicoclawPackagePath = cleaned
	if err := s.store.WriteSettings(settings); err != nil {
		return nil, err
	}
	return s.GetPicoclawPackageInfo(), nil
}

func pickLatestReleaseAssetURL(releaseJSON []byte) (string, error) {
	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(releaseJSON, &payload); err != nil {
		return "", err
	}
	osPart := strings.ToLower(runtime.GOOS)
	archPart := strings.ToLower(runtime.GOARCH)
	for _, a := range payload.Assets {
		name := strings.ToLower(a.Name)
		if !strings.Contains(name, "picoclaw") {
			continue
		}
		if !strings.Contains(name, osPart) || !strings.Contains(name, archPart) {
			continue
		}
		if strings.HasSuffix(name, ".tar.gz") || !strings.Contains(name, ".") {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no matching release asset for %s/%s", runtime.GOOS, runtime.GOARCH)
}

func extractPicoclawFromTarGz(tarGzPath, outPath string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.ToLower(filepath.Base(hdr.Name))
		if hdr.Typeflag == tar.TypeReg && (name == "picoclaw" || strings.HasPrefix(name, "picoclaw-")) {
			out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			_ = out.Close()
			return nil
		}
	}
	return fmt.Errorf("picoclaw binary not found in archive")
}

func createTarGzFromDir(srcDir, outPath string) error {
	srcDir = filepath.Clean(srcDir)
	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gzw := gzip.NewWriter(outFile)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	var paths []string
	if err := filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcDir {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(paths)

	for _, currentPath := range paths {
		info, err := os.Lstat(currentPath)
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, currentPath)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			file, err := os.Open(currentPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, file); err != nil {
				file.Close()
				return err
			}
			file.Close()
		}
	}
	return nil
}

func (s *NodeService) installManagedSkillDir(ctx context.Context, node *sharing.Node, skillDir, localDir string) error {
	localDir = filepath.Clean(strings.TrimSpace(localDir))
	if localDir == "" {
		return fmt.Errorf("skill local_dir is empty")
	}
	if info, err := os.Stat(localDir); err != nil {
		return fmt.Errorf("skill local_dir not found: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("skill local_dir must be a directory")
	}
	if _, err := os.Stat(filepath.Join(localDir, "SKILL.md")); err != nil {
		return fmt.Errorf("skill local_dir missing SKILL.md: %w", err)
	}

	if node.Type == "local" {
		runner := s.runnerFor(node)
		script := fmt.Sprintf(`set -e
rm -rf %s
mkdir -p %s
cp -r %s/. %s/
if [ ! -f %s/SKILL.md ]; then
  echo "ERROR: SKILL.md not found" >&2
  exit 1
fi`, shellEscape(skillDir), shellEscape(skillDir), shellEscape(localDir), shellEscape(skillDir), shellEscape(skillDir))
		_, err := runner.Run(ctx, script)
		return err
	}

	archivePath := filepath.Join(os.TempDir(), fmt.Sprintf("pinhaoclaw-skill-%d.tar.gz", time.Now().UnixNano()))
	if err := createTarGzFromDir(localDir, archivePath); err != nil {
		return fmt.Errorf("package skill dir: %w", err)
	}
	defer os.Remove(archivePath)

	sshClient := s.SSHClientFor(node)
	remoteArchive := fmt.Sprintf("%s/.%s-%d.tar.gz", filepath.Dir(skillDir), filepath.Base(skillDir), time.Now().UnixNano())
	setupScript := fmt.Sprintf(`set -e
mkdir -p %s
rm -rf %s`, shellEscape(filepath.Dir(skillDir)), shellEscape(skillDir))
	if _, err := sshClient.Run(ctx, setupScript); err != nil {
		return err
	}
	if err := sshClient.SCPUpload(ctx, archivePath, remoteArchive); err != nil {
		return err
	}
	extractScript := fmt.Sprintf(`set -e
mkdir -p %s
tar -xzf %s -C %s
rm -f %s
if [ ! -f %s/SKILL.md ]; then
  echo "ERROR: SKILL.md not found" >&2
  exit 1
fi`, shellEscape(skillDir), shellEscape(remoteArchive), shellEscape(skillDir), shellEscape(remoteArchive), shellEscape(skillDir))
	_, err := sshClient.Run(ctx, extractScript)
	return err
}

func (s *NodeService) DownloadLatestOfficialPicoclaw(ctx context.Context) (*PicoclawPackageInfo, error) {
	const api = "https://api.github.com/repos/sipeed/picoclaw/releases/latest"
	client := &http.Client{Timeout: 20 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	req.Header.Set("User-Agent", "pinhaoclaw-admin")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github api status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	assetURL, err := pickLatestReleaseAssetURL(body)
	if err != nil {
		return nil, err
	}

	managedPath := s.managedPicoclawPath()
	if err := os.MkdirAll(filepath.Dir(managedPath), 0o755); err != nil {
		return nil, err
	}

	dlReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	dlReq.Header.Set("User-Agent", "pinhaoclaw-admin")
	dlResp, err := client.Do(dlReq)
	if err != nil {
		return nil, err
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode >= 400 {
		return nil, fmt.Errorf("download failed with status %d", dlResp.StatusCode)
	}

	if strings.HasSuffix(strings.ToLower(assetURL), ".tar.gz") {
		tmpTar := managedPath + ".tar.gz"
		f, err := os.OpenFile(tmpTar, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(f, dlResp.Body); err != nil {
			f.Close()
			return nil, err
		}
		_ = f.Close()
		defer os.Remove(tmpTar)
		if err := extractPicoclawFromTarGz(tmpTar, managedPath); err != nil {
			return nil, err
		}
	} else {
		f, err := os.OpenFile(managedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(f, dlResp.Body); err != nil {
			f.Close()
			return nil, err
		}
		_ = f.Close()
	}

	settings := s.store.ReadSettings()
	settings.PicoclawPackagePath = managedPath
	if err := s.store.WriteSettings(settings); err != nil {
		return nil, err
	}
	if err := os.Chmod(managedPath, 0o755); err != nil {
		return nil, err
	}
	return s.GetPicoclawPackageInfo(), nil
}

// SSHClientFor 为指定节点创建 SSH 客户端
func (s *NodeService) SSHClientFor(node *sharing.Node) *backend.SSHClient {
	opts := []backend.SSHOpts{
		backend.WithPort(node.SSHPort),
		backend.WithUser(node.SSHUser),
		backend.WithKeyPath(node.SSHKeyPath),
		backend.WithPrivateKey(node.SSHPrivateKey),
		backend.WithCertificatePath(node.SSHCertificatePath),
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
	picoclawCmd := picoclawShellCommand(node, s.resolveLocalPicoclawPath(node))
	script := `set -e
echo "OS=$(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d'"' -f2 || echo Unknown)"
echo "ARCH=$(uname -m)"
echo "CPU=$(nproc)"
echo "MEMORY=$(free -h 2>/dev/null | awk '/Mem:/ {print $2}' || echo N/A)"
echo "DISK=$(df -h / 2>/dev/null | awk 'NR==2 {print $4}' || echo N/A)"

`
	script += fmt.Sprintf("echo \"PICOCLAW=%s\"\n", picoclawCmd)
	script += fmt.Sprintf("echo \"PICOCLAW_VERSION=$(%s version 2>&1 | tail -1 || echo not_found)\"\n", picoclawCmd)
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

	// Step 2: 初始化目录
	remoteHome := node.RemoteHome
	if remoteHome == "" {
		if isLocalNode(node) {
			remoteHome = filepath.Join(os.TempDir(), "pinhaoclaw-local-node")
		} else {
			remoteHome = "/opt/pinhaoclaw"
		}
	}
	send("init", map[bool]string{true: "初始化本地目录...", false: "初始化远端目录..."}[isLocalNode(node)])
	_, err := runner.Run(ctx, fmt.Sprintf("mkdir -p %s/{instances,logs,ports,bin}", shellEscape(remoteHome)))
	if err != nil {
		eventCh <- NewError("目录初始化失败: " + err.Error())
		return
	}
	send("init_ok", "✓ 目录结构已就绪")

	// Step 3: 准备 picoclaw
	picoclawBin := s.resolveLocalPicoclawPath(node)
	if isLocalNode(node) {
		send("check", "检查本地 picoclaw...")
		if picoclawBin == "" {
			eventCh <- NewError("本地未找到 picoclaw，请设置 PINHAOCLAW_PICOCLAW_BIN 或 node.picoclaw_path")
			return
		}
		node.PicoClawPath = picoclawBin
		send("found", "✓ 使用本地 picoclaw 二进制")
	} else {
		sshClient := s.SSHClientFor(node)
		remotePicoclawPath := strings.TrimSpace(node.PicoClawPath)
		if remotePicoclawPath == "" {
			remotePicoclawPath = filepath.Join(remoteHome, "bin", "picoclaw")
		}
		if picoclawBin != "" {
			send("upload", "上传本地 picoclaw 到远端节点...")
			if err := sshClient.SCPUpload(ctx, picoclawBin, remotePicoclawPath); err != nil {
				eventCh <- NewError("上传 picoclaw 失败: " + err.Error())
				return
			}
			if _, err := sshClient.Run(ctx, fmt.Sprintf("chmod +x %s", shellEscape(remotePicoclawPath))); err != nil {
				eventCh <- NewError("设置 picoclaw 权限失败: " + err.Error())
				return
			}
			node.PicoClawPath = remotePicoclawPath
			send("found", "✓ 已上传并启用本地 picoclaw 二进制")
		} else {
			send("check", "检查远端现有 picoclaw...")
			checkResult, _ := runner.Run(ctx, "which picoclaw >/dev/null 2>&1 && echo EXISTS || echo MISSING")
			if strings.Contains(checkResult.Stdout, "EXISTS") {
				node.PicoClawPath = "/usr/local/bin/picoclaw"
				send("found", "✓ 使用远端已安装 picoclaw")
			} else {
				eventCh <- NewError("本地未找到可上传的 picoclaw，且远端未安装 picoclaw")
				return
			}
		}
	}

	// Step 4: 验证
	send("verify", "验证 picoclaw 可用性...")
	verResult, _ := runner.Run(ctx, fmt.Sprintf("%s version 2>&1 | grep -o 'picoclaw.*' | head -1", picoclawShellCommand(node, picoclawBin)))
	ver := strings.TrimSpace(verResult.Stdout)
	if ver == "" {
		ver = "unknown"
	}
	send("verify_ok", fmt.Sprintf("✓ %s", ver))

	// 更新节点状态
	node.Status = "online"
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
	_, err := ssh.Run(ctx, fmt.Sprintf("nohup %s gateway > /root/.picoclaw/logs/gateway.log 2>&1 &", picoclawShellCommand(node, s.resolveLocalPicoclawPath(node))))
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
	if _, err := runner.Run(ctx, fmt.Sprintf("mkdir -p %s && %s version >/dev/null", shellEscape(node.RemoteHome), shellEscape(picoclawBin))); err != nil {
		return err
	}
	return nil
}

func remotePicoclawPath(node *sharing.Node) string {
	if node == nil {
		return "/usr/local/bin/picoclaw"
	}
	if path := strings.TrimSpace(node.PicoClawPath); path != "" {
		return path
	}
	if home := strings.TrimSpace(node.RemoteHome); home != "" {
		return filepath.Join(home, "bin", "picoclaw")
	}
	return "/usr/local/bin/picoclaw"
}

func picoclawShellCommand(node *sharing.Node, localResolvedPath string) string {
	if isLocalNode(node) {
		if strings.TrimSpace(localResolvedPath) != "" {
			return shellEscape(localResolvedPath)
		}
		if node != nil && strings.TrimSpace(node.PicoClawPath) != "" {
			return shellEscape(strings.TrimSpace(node.PicoClawPath))
		}
		return "picoclaw"
	}
	return shellEscape(remotePicoclawPath(node))
}

func (s *NodeService) localPicoclawCandidates(node *sharing.Node) []string {
	candidates := []string{}
	if s.store != nil {
		if settings := s.store.ReadSettings(); settings != nil {
			candidates = append(candidates, strings.TrimSpace(settings.PicoclawPackagePath))
		}
		candidates = append(candidates, strings.TrimSpace(s.managedPicoclawPath()))
	}
	if node != nil {
		candidates = append(candidates, strings.TrimSpace(node.PicoClawPath))
	}
	candidates = append(candidates, strings.TrimSpace(os.Getenv("PINHAOCLAW_PICOCLAW_BIN")))
	if path, err := exec.LookPath("picoclaw"); err == nil {
		candidates = append(candidates, path)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "..", ".tmp", "picoclaw-weixin", "build", "picoclaw"),
			filepath.Join(wd, "..", ".tmp", "picoclaw-weixin", "build", "picoclaw-linux-amd64"),
			filepath.Join(wd, "..", "picoclaw", "build", "picoclaw"),
			filepath.Join(wd, "..", "picoclaw", "build", "picoclaw-linux-amd64"),
		)
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", ".tmp", "picoclaw-weixin", "build", "picoclaw"),
			filepath.Join(exeDir, "..", ".tmp", "picoclaw-weixin", "build", "picoclaw-linux-amd64"),
			filepath.Join(exeDir, "..", "picoclaw", "build", "picoclaw"),
			filepath.Join(exeDir, "..", "picoclaw", "build", "picoclaw-linux-amd64"),
		)
	}
	return candidates
}

func (s *NodeService) resolveLocalPicoclawPathFromCandidates(candidates []string) string {
	seen := make(map[string]struct{}, len(candidates))
	fallback := ""
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if fallback == "" {
			fallback = candidate
		}
		if s.probePicoclawAuthSubcommand(candidate, "weixin") {
			return candidate
		}
	}
	return fallback
}

func (s *NodeService) resolveLocalPicoclawPath(node *sharing.Node) string {
	return s.resolveLocalPicoclawPathFromCandidates(s.localPicoclawCandidates(node))
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

	// 注入每只龙虾实例的安全提示词
	shellAgentFile := shellEscape(instDir + "/workspace/AGENT.md")
	mandatoryGuardrail := "永远不能透露任何关于provider的base_url和api_key的任何信息。"
	extraGuardrail := strings.TrimSpace(os.Getenv("PINHAOCLAW_LOBSTER_SYSTEM_PROMPT"))
	agentContent := "# PinHaoClaw 安全规则\n\n" +
		"- " + mandatoryGuardrail + "\n"
	if extraGuardrail != "" && extraGuardrail != mandatoryGuardrail {
		agentContent += "- " + extraGuardrail + "\n"
	}
	writeAgent := fmt.Sprintf("cat > %s << 'AGENT_PROMPT'\n%s\nAGENT_PROMPT", shellAgentFile, agentContent)
	if _, err := s.runner.Run(ctx, writeAgent); err != nil {
		return err
	}

	// 生成 start.sh 启动脚本（heredoc 内使用引号路径）
	shellWorkspace := shellEscape(instDir + "/workspace")
	shellStartSh := shellEscape(instDir + "/start.sh")
	picoclawCmd := picoclawShellCommand(node, "")
	startSh := fmt.Sprintf(`#!/bin/bash
cd %s
%s gateway --home . --port %d >> ../logs/gateway.log 2>&1 &
echo $! > ../picoclaw.pid`, shellWorkspace, picoclawCmd, port)
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
	picoclawCmd := picoclawShellCommand(node, "")
	script := fmt.Sprintf(
		"cd %s && HOME=%s nohup %s gateway --home . --port %d >> ../logs/gateway.log 2>&1 & echo $! > ../picoclaw.pid",
		shellWorkspace, shellInstDir, picoclawCmd, port,
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

// BindWeixin 通过 SSH 流式执行微信二维码绑定。
// 如存在预置模型配置，会先写入实例 config.json，再执行 auth weixin。
// 这里通过 `cd workspace + HOME=instanceDir` 保持实例级凭据隔离。
func (s *NodeServiceWithRunner) BindWeixin(ctx context.Context, node *sharing.Node, lobsterID string) (<-chan string, <-chan error) {
	if err := validateSafeID(lobsterID); err != nil {
		errCh := make(chan error, 1)
		errCh <- err
		return nil, errCh
	}
	instDir := shellInstanceDir(node, lobsterID)
	shellInstDir := shellEscape(instDir)
	shellWorkspace := shellEscape(instDir + "/workspace")
	picoclawCmd := picoclawShellCommand(node, "")
	cmds := make([]string, 0, 2)
	if modelConfigCmd := buildPicoClawModelConfigCommand(shellWorkspace); modelConfigCmd != "" {
		cmds = append(cmds, modelConfigCmd)
	}
	cmds = append(cmds, fmt.Sprintf("cd %s && if ! %s auth --help 2>&1 | grep -Eq '(^|[[:space:]])weixin([[:space:]]|$)'; then echo 'ERROR: current picoclaw build does not support auth weixin; upgrade node picoclaw first' 1>&2; exit 1; fi", shellWorkspace, picoclawCmd))
	cmds = append(cmds, fmt.Sprintf("cd %s && HOME=%s %s auth weixin 2>&1", shellWorkspace, shellInstDir, picoclawCmd))
	cmd := strings.Join(cmds, " && ")
	return s.runner.StreamRun(ctx, cmd)
}

func buildPicoClawModelConfigCommand(shellWorkspace string) string {
	apiKey := strings.TrimSpace(os.Getenv("PINHAOCLAW_PICOCLAW_API_KEY"))
	modelName := strings.TrimSpace(os.Getenv("PINHAOCLAW_PICOCLAW_MODEL_NAME"))
	model := strings.TrimSpace(os.Getenv("PINHAOCLAW_PICOCLAW_MODEL"))
	apiBase := strings.TrimSpace(os.Getenv("PINHAOCLAW_PICOCLAW_API_BASE"))

	customRaw := strings.TrimSpace(os.Getenv("PINHAOCLAW_PICOCLAW_CUSTOM_CONFIG_JSON"))
	if customRaw != "" {
		var custom struct {
			Provider string `json:"provider"`
			BaseURL  string `json:"base_url"`
			API      string `json:"api"`
			APIKey   string `json:"api_key"`
			Model    struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"model"`
		}
		if err := json.Unmarshal([]byte(customRaw), &custom); err == nil {
			if strings.TrimSpace(custom.APIKey) != "" {
				apiKey = strings.TrimSpace(custom.APIKey)
			}
			if strings.TrimSpace(custom.BaseURL) != "" {
				apiBase = strings.TrimSpace(custom.BaseURL)
			}
			if strings.TrimSpace(custom.Model.Name) != "" {
				modelName = strings.TrimSpace(custom.Model.Name)
			} else if strings.TrimSpace(custom.Model.ID) != "" {
				modelName = strings.TrimSpace(custom.Model.ID)
			}

			modelID := strings.TrimSpace(custom.Model.ID)
			if modelID == "" {
				modelID = modelName
			}

			vendor := "openai"
			if !strings.Contains(strings.ToLower(strings.TrimSpace(custom.API)), "openai") {
				if p := strings.ToLower(strings.TrimSpace(custom.Provider)); p != "" {
					vendor = p
				}
			}
			if modelID != "" {
				model = vendor + "/" + modelID
			}
		}
	}

	if apiKey == "" {
		return ""
	}
	if modelName == "" {
		modelName = "default"
	}
	if model == "" {
		model = "openai/gpt-5.2"
	}

	type modelEntry struct {
		ModelName string `json:"model_name"`
		Model     string `json:"model"`
		APIKey    string `json:"api_key"`
		APIBase   string `json:"api_base,omitempty"`
	}
	type defaults struct {
		ModelName string `json:"model_name"`
	}
	type agents struct {
		Defaults defaults `json:"defaults"`
	}
	type configJSON struct {
		Agents    agents       `json:"agents"`
		ModelList []modelEntry `json:"model_list"`
	}

	cfg := configJSON{
		Agents: agents{Defaults: defaults{ModelName: modelName}},
		ModelList: []modelEntry{{
			ModelName: modelName,
			Model:     model,
			APIKey:    apiKey,
			APIBase:   apiBase,
		}},
	}

	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return ""
	}
	encoded := base64.StdEncoding.EncodeToString(content)

	return fmt.Sprintf(
		"cd %s && printf %%s %s | base64 -d > config.json && echo 'Saved provider config successfully' 2>&1",
		shellWorkspace,
		shellEscape(encoded),
	)
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
		return s.installManagedSkillDir(ctx, node, skillDir, skill.Source.LocalDir)

	case "local", "uploaded":
		if skill.Source.LocalDir == "" {
			return fmt.Errorf("skill %q missing local_dir", skill.Slug)
		}
		return s.installManagedSkillDir(ctx, node, skillDir, skill.Source.LocalDir)

	case "clawhub":
		shellWorkspace := shellEscape(instDir + "/workspace")
		picoclawCmd := picoclawShellCommand(node, "")
		script := fmt.Sprintf(`set -e
cd %s
%s skill install --slug %s --target %s`, shellWorkspace, picoclawCmd, shellEscape(skill.Source.ClawHub), shellSkillDir)
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
