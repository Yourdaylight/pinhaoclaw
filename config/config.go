package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	ShareClawHome  string
	AdminPassword  string
	AdminPath      string // 管理后台隐藏路径，如 /manage-xxxx
	RemoteHost     string
	RemoteSSHPort  int
	RemoteUser     string
	RemoteKeyPath  string
	RemotePassword string
	RemoteHome     string
	RemoteRegion   string
}

func Load() *Config {
	homeDir, _ := os.UserHomeDir()

	cfg := &Config{
		ShareClawHome:  envOr("PINHAOCLAW_HOME", filepath.Join(homeDir, ".pinhaoclaw")),
		AdminPassword:  os.Getenv("PINHAOCLAW_ADMIN_PASSWORD"),
		AdminPath:      envOr("PINHAOCLAW_ADMIN_PATH", ""),
		RemoteHost:     os.Getenv("PINHAOCLAW_REMOTE_HOST"),
		RemoteUser:     envOr("PINHAOCLAW_REMOTE_USER", "root"),
		RemoteKeyPath:  os.Getenv("PINHAOCLAW_REMOTE_KEY_PATH"),
		RemotePassword: os.Getenv("PINHAOCLAW_REMOTE_PASSWORD"),
		RemoteHome:     envOr("PINHAOCLAW_REMOTE_HOME", "/opt/pinhaoclaw"),
		RemoteRegion:   envOr("PINHAOCLAW_REMOTE_REGION", "华南"),
		RemoteSSHPort:  22,
	}

	if v := os.Getenv("PINHAOCLAW_REMOTE_SSH_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.RemoteSSHPort)
	}

	return cfg
}

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
