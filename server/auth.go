package server

import (
	"crypto/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const adminTokenTTL = 7 * 24 * time.Hour

var (
	adminTokens = make(map[string]time.Time)
	tokenMu     sync.RWMutex
)

// generateToken 生成随机 token
func generateAdminToken() string {
	b := make([]byte, 44)
	rand.Read(b)
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	result := make([]byte, 44)
	for i := range result {
		result[i] = chars[int(b[i])%len(chars)]
	}
	return string(result)
}

// AdminAuth 虾主认证管理器
type AdminAuth struct {
	password string
}

// NewAdminAuth 创建认证器
func NewAdminAuth(password string) *AdminAuth {
	return &AdminAuth{password: password}
}

// Login 验证密码并返回 token
func (a *AdminAuth) Login(password string) (string, bool) {
	if a.password == "" {
		// 未配置密码时拒绝所有登录，强制要求设置 SHARECLAW_ADMIN_PASSWORD
		return "", false
	}
	if password == "" {
		return "", false
	}
	if password == a.password {
		return generateAdminToken(), true
	}
	return "", false
}

// Check 检查请求中的 token 是否有效
func (a *AdminAuth) Check(c *gin.Context) bool {
	if a.password == "" {
		return false // 未配置密码，全部拒绝
	}
	// 只从 Header 读取，不允许 Query 参数（防止 token 泄露到日志/浏览器历史）
	token := c.GetHeader("X-Admin-Token")
	if token == "" {
		return false
	}

	tokenMu.RLock()
	expire, ok := adminTokens[token]
	tokenMu.RUnlock()
	if !ok || time.Now().After(expire) {
		tokenMu.Lock()
		delete(adminTokens, token)
		tokenMu.Unlock()
		return false
	}
	return true
}

// StoreToken 存储 token
func (a *AdminAuth) StoreToken(token string) {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	adminTokens[token] = time.Now().Add(adminTokenTTL)
}

// HasPassword 是否设置了密码
func (a *AdminAuth) HasPassword() bool { return a.password != "" }

// RequireAdmin 要求虾主身份的中间件
func (a *AdminAuth) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.Check(c) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "请先登录虾主管理面板",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
