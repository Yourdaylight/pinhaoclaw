package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type AuthMode string

const (
	AuthModeInvite  AuthMode = "invite"
	AuthModeSidecar AuthMode = "sidecar"
)

type Config struct {
	ShareClawHome   string
	FrontendDir     string
	AuthMode        AuthMode
	AdminPassword   string
	AdminPath       string
	RemoteHost      string
	RemoteSSHPort   int
	RemoteUser      string
	RemoteKeyPath   string
	RemotePassword  string
	RemoteHome      string
	RemoteRegion    string
	PublicOrigin    string
	AuthSidecarURL  string // e.g. http://localhost:9098 or https://auth.example.com
	CasdoorEndpoint string // e.g. https://auth.example.com
}

func Load() (*Config, error) {
	// Load .env file first (real env vars take precedence)
	if err := loadDotEnv(); err != nil {
		return nil, fmt.Errorf("加载 .env 文件失败: %w", err)
	}

	homeDir, _ := os.UserHomeDir()

	sshPort, err := envInt("PINHAOCLAW_REMOTE_SSH_PORT", 22)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		ShareClawHome:   cleanPath(envOr("PINHAOCLAW_HOME", filepath.Join(homeDir, ".pinhaoclaw"))),
		FrontendDir:     cleanPath(envOr("PINHAOCLAW_FRONTEND_DIR", filepath.Join("pinhaoclaw-frontend", "dist", "build", "h5"))),
		AdminPassword:   os.Getenv("PINHAOCLAW_ADMIN_PASSWORD"),
		AdminPath:       normalizeSlashPath(envOr("PINHAOCLAW_ADMIN_PATH", "")),
		RemoteHost:      os.Getenv("PINHAOCLAW_REMOTE_HOST"),
		RemoteUser:      envOr("PINHAOCLAW_REMOTE_USER", "root"),
		RemoteKeyPath:   cleanPath(os.Getenv("PINHAOCLAW_REMOTE_KEY_PATH")),
		RemotePassword:  os.Getenv("PINHAOCLAW_REMOTE_PASSWORD"),
		RemoteHome:      cleanPath(envOr("PINHAOCLAW_REMOTE_HOME", "/opt/pinhaoclaw")),
		RemoteRegion:    envOr("PINHAOCLAW_REMOTE_REGION", "华南"),
		RemoteSSHPort:   sshPort,
		PublicOrigin:    strings.TrimRight(envOr("PINHAOCLAW_PUBLIC_ORIGIN", "http://localhost:9000"), "/"),
		AuthSidecarURL:  strings.TrimRight(envOr("PINHAOCLAW_AUTH_SIDECAR_URL", ""), "/"),
		CasdoorEndpoint: strings.TrimRight(envOr("PINHAOCLAW_CASDOOR_ENDPOINT", ""), "/"),
	}

	cfg.AuthMode, err = resolveAuthMode(os.Getenv("PINHAOCLAW_AUTH_MODE"), cfg.AuthSidecarURL != "")
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	var problems []string
	if c.ShareClawHome == "" {
		problems = append(problems, "PINHAOCLAW_HOME 不能为空")
	}
	if c.FrontendDir == "" {
		problems = append(problems, "PINHAOCLAW_FRONTEND_DIR 不能为空")
	}
	if c.AdminPath == "/" {
		problems = append(problems, "PINHAOCLAW_ADMIN_PATH 不能是根路径 /")
	}
	if c.AdminPath != "" && strings.ContainsAny(c.AdminPath, " \t\n") {
		problems = append(problems, "PINHAOCLAW_ADMIN_PATH 不能包含空白字符")
	}
	if err := validateHTTPURL("PINHAOCLAW_PUBLIC_ORIGIN", c.PublicOrigin); err != nil {
		problems = append(problems, err.Error())
	}
	if c.AuthMode == AuthModeSidecar {
		if c.AuthSidecarURL == "" {
			problems = append(problems, "PINHAOCLAW_AUTH_SIDECAR_URL 不能为空（当前认证模式为 sidecar）")
		} else if err := validateHTTPURL("PINHAOCLAW_AUTH_SIDECAR_URL", c.AuthSidecarURL); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf(strings.Join(problems, "；"))
	}
	return nil
}

func (c *Config) PrepareForStart() error {
	if abs, err := filepath.Abs(c.ShareClawHome); err == nil {
		c.ShareClawHome = abs
	}
	if abs, err := filepath.Abs(c.FrontendDir); err == nil {
		c.FrontendDir = abs
	}
	if err := os.MkdirAll(c.ShareClawHome, 0o755); err != nil {
		return fmt.Errorf("创建 PINHAOCLAW_HOME 失败: %w", err)
	}
	probeFile := filepath.Join(c.ShareClawHome, ".startup-write-check")
	if err := os.WriteFile(probeFile, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("PINHAOCLAW_HOME 不可写: %w", err)
	}
	_ = os.Remove(probeFile)
	indexFile := filepath.Join(c.FrontendDir, "index.html")
	if _, err := os.Stat(indexFile); err != nil {
		return fmt.Errorf("未找到前端构建产物: %s", indexFile)
	}
	return nil
}

func (c *Config) SidecarEnabled() bool {
	return c.AuthMode == AuthModeSidecar
}

func resolveAuthMode(raw string, hasSidecarURL bool) (AuthMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		if hasSidecarURL {
			return AuthModeSidecar, nil
		}
		return AuthModeInvite, nil
	case string(AuthModeInvite):
		return AuthModeInvite, nil
	case string(AuthModeSidecar):
		return AuthModeSidecar, nil
	default:
		return "", fmt.Errorf("PINHAOCLAW_AUTH_MODE 仅支持 invite / sidecar / auto，当前值: %s", raw)
	}
}

func validateHTTPURL(name, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%s 不能为空", name)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s 不是合法 URL: %w", name, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s 必须使用 http 或 https", name)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s 缺少 host", name)
	}
	return nil
}

func normalizeSlashPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return "/" + strings.Trim(raw, "/")
}

func cleanPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return filepath.Clean(raw)
}

func envInt(key string, defaultVal int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultVal, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s 必须是整数: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s 必须大于 0", key)
	}
	return value, nil
}

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
