package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pinhaoclaw/pinhaoclaw/config"
	"github.com/pinhaoclaw/pinhaoclaw/server"
)

func main() {
	port := flag.Int("p", envInt("PORT", 9000), "监听端口")
	host := flag.String("H", "0.0.0.0", "监听地址")
	version := flag.Bool("version", false, "显示版本信息")
	flag.Parse()

	if *version {
		fmt.Println("PinHaoClaw v1.0.0")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载启动配置失败: %v", err)
	}

	// 如果未配置 AdminPath，自动生成随机路径
	if cfg.AdminPath == "" {
		cfg.AdminPath = "/mgr-" + randomSuffix(8)
		log.Printf("[PinHaoClaw] 📋 自动生成管理后台路径: %s", cfg.AdminPath)
	}
	if err := cfg.PrepareForStart(); err != nil {
		log.Fatalf("启动前检查失败: %v", err)
	}

	log.Printf("[PinHaoClaw] 🦞 启动中...")
	log.Printf("  监听地址: http://%s:%d", *host, *port)
	log.Printf("  数据目录: %s", cfg.ShareClawHome)
	log.Printf("  前端目录: %s", cfg.FrontendDir)
	log.Printf("  认证模式: %s", cfg.AuthMode)
	if cfg.SidecarEnabled() {
		log.Printf("[PinHaoClaw] 🔐 统一认证已启用: sidecar=%s", cfg.AuthSidecarURL)
	}

	if cfg.AdminPassword == "" {
		log.Printf("[PinHaoClaw] ⚠️  未设置 PINHAOCLAW_ADMIN_PASSWORD，管理后台登录已禁用")
	} else {
		log.Printf("[PinHaoClaw] 🔐 管理后台入口: http://%s:%d%s", *host, *port, cfg.AdminPath)
	}

	app := server.NewApp(cfg)

	if err := app.Run(fmt.Sprintf("%s:%d", *host, *port)); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}

func randomSuffix(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, n)
	for i := range result {
		result[i] = chars[int(b[i])%len(chars)]
	}
	return string(result)
}

func envInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		return defaultVal
	}
	return n
}
