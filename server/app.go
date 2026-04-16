package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/pinhaoclaw/pinhaoclaw/claw"
	"github.com/pinhaoclaw/pinhaoclaw/config"
	"github.com/pinhaoclaw/pinhaoclaw/sharing"
)

type App struct {
	cfg     *config.Config
	store   *sharing.Store
	nodeSvc *claw.NodeService
	auth    *AdminAuth
	sidecar *SidecarClient
	router  *gin.Engine

	// userOpMu protects check-and-act operations (lobster creation, invite validation, node count)
	userOpMu sync.Mutex
}

func NewApp(cfg *config.Config) *App {
	store := sharing.NewStore(cfg.ShareClawHome)
	app := &App{
		cfg:     cfg,
		store:   store,
		nodeSvc: claw.NewNodeService(store),
		auth:    NewAdminAuth(cfg.AdminPassword),
		sidecar: NewSidecarClient(cfg),
	}
	app.setupRouter()

	// 如果有环境变量配置的节点，自动注册
	if cfg.RemoteHost != "" {
		s := store.ReadSettings()
		nodes, _ := store.ReadNodes()
		found := false
		for _, n := range nodes {
			if n.Host == cfg.RemoteHost {
				found = true
				break
			}
		}
		if !found {
			node := &sharing.Node{
				ID:           "node_" + shortID(),
				Type:         "ssh",
				Name:         "默认节点",
				Host:         cfg.RemoteHost,
				SSHPort:      cfg.RemoteSSHPort,
				SSHUser:      cfg.RemoteUser,
				SSHKeyPath:   cfg.RemoteKeyPath,
				SSHPassword:  cfg.RemotePassword,
				Region:       cfg.RemoteRegion,
				Status:       "online",
				MaxLobsters:  10,
				PicoClawPath: "/usr/local/bin/picoclaw",
				RemoteHome:   cfg.RemoteHome,
				CreatedAt:    time.Now().Format("2006-01-02 15:04:05"),
			}
			store.SaveNode(node)
		}
		_ = s
	}

	return app
}

func (a *App) Run(addr string) error { return a.router.Run(addr) }

func (a *App) setupRouter() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(corsMiddleware())

	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	api := r.Group("/api")

	// ── 用户认证 ──
	api.GET("/auth/config", a.handleAuthConfig)
	api.POST("/auth/login", a.handleUserLogin)
	api.GET("/auth/me", a.requireUser(), a.handleMe)
	api.GET("/regions", a.requireUser(), a.handleListRegions)
	api.GET("/qrcode", a.handleQRCode)

	// ── Sidecar 登录代理（浏览器 → pinhaoclaw → sidecar）──
	if a.cfg.SidecarEnabled() {
		api.GET("/auth/sidecar/login", a.handleSidecarLogin)
		api.GET("/auth/sidecar/callback", a.handleSidecarCallback)
		api.POST("/auth/sidecar/logout", a.requireUser(), a.handleSidecarLogout)
		api.GET("/auth/sidecar/logout", a.handleSidecarLogoutPage)
		api.GET("/auth/sidecar/logout-complete", a.handleSidecarLogoutComplete)
	}

	// ── 用户龙虾操作 ──
	user := api.Group("/lobsters")
	user.Use(a.requireUser())
	user.GET("", a.handleListMyLobsters)
	user.POST("", a.handleCreateLobster)
	user.GET("/:id", a.handleGetLobster)
	user.GET("/:id/bind", a.handleBindWeixin) // SSE，H5 端使用
	user.POST("/:id/stop", a.handleStopLobster)
	user.POST("/:id/start", a.handleStartLobster)
	user.DELETE("/:id", a.handleDeleteLobster)

	// ── WebSocket（小程序端使用）──
	r.GET("/ws/bind/:id", a.handleBindWeixinWS)

	// ── Skill 库（用户浏览） ──
	api.GET("/skills", a.requireUser(), a.handleListSkills)
	api.GET("/skills/:slug", a.requireUser(), a.handleGetSkill)

	// ── 龙虾 Skill 管理 ──
	user.GET("/:id/skills", a.handleListLobsterSkills)
	user.POST("/:id/skills", a.handleInstallSkill)
	user.DELETE("/:id/skills/:slug", a.handleUninstallSkill)

	// ── 管理员 ──
	api.POST("/admin/login", a.handleAdminLogin)
	admin := api.Group("/admin")
	admin.Use(a.auth.RequireAdmin())
	admin.GET("/overview", a.handleAdminOverview)
	admin.GET("/lobsters", a.handleAdminLobsters)
	admin.GET("/nodes", a.handleListNodes)
	admin.POST("/nodes", a.handleAddNode)
	admin.DELETE("/nodes/:id", a.handleDeleteNode)
	admin.POST("/nodes/:id/deploy", a.handleDeployNode)
	admin.POST("/nodes/:id/test", a.handleTestNode)
	admin.GET("/invites", a.handleListInvites)
	admin.POST("/invites", a.handleCreateInvite)
	admin.DELETE("/invites/:code", a.handleDeleteInvite)
	admin.GET("/settings", a.handleGetSettings)
	admin.PUT("/settings", a.handleUpdateSettings)

	// ── 管理员 Skill 库管理 ──
	admin.GET("/skills", a.handleAdminListSkills)
	admin.POST("/skills", a.handleAdminCreateSkill)
	admin.PUT("/skills/:slug", a.handleAdminUpdateSkill)
	admin.DELETE("/skills/:slug", a.handleAdminDeleteSkill)

	// ── 管理后台隐藏入口验证（无需登录）──
	// 前端管理页加载时调用，确认当前访问路径匹配 AdminPath 配置
	r.GET("/api/admin/gate", a.handleAdminGate)

	// ── 管理后台隐藏路径路由（AdminPath 如 /mgr-x7Kp9qZ）──
	// 注意：此段必须在 frontendDir 定义之后（见下方静态文件区域）
	// 此处仅注册路由占位，实际逻辑在下方 frontendDir 声明后处理

	// ── 静态文件（H5 前端产物）──
	frontendDir := a.cfg.FrontendDir
	frontendIndex := frontendDir + "/index.html"
	r.Static("/assets", frontendDir+"/assets")
	r.StaticFile("/favicon.ico", frontendDir+"/favicon.ico")

	serveFrontendIndex := func(c *gin.Context) {
		c.File(frontendIndex)
	}

	// 管理后台隐藏路径（在此处定义，因为需要 frontendDir）
	if a.cfg.AdminPath != "" {
		adminPath := strings.TrimPrefix(a.cfg.AdminPath, "/")
		r.GET("/"+adminPath, serveFrontendIndex)
		r.GET("/"+adminPath+"/*filepath", serveFrontendIndex)
	}

	r.NoRoute(func(c *gin.Context) {
		if shouldBlockFrontendFallback(c.Request.URL.Path) {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		// 所有未匹配前端路由回退到 index.html（SPA 路由支持）
		if !strings.HasPrefix(c.Request.URL.Path, "/api") && !strings.HasPrefix(c.Request.URL.Path, "/ws") {
			serveFrontendIndex(c)
			return
		}
		c.JSON(404, gin.H{"error": "not found"})
	})

	a.router = r
}

// ── 用户认证中间件 ──────────────────────────────────

func (a *App) requireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := userTokenFromRequest(c.Request, strings.HasPrefix(c.Request.URL.Path, "/ws/") || strings.HasSuffix(c.Request.URL.Path, "/bind"))
		if token == "" {
			c.JSON(401, gin.H{"error": "请先登录"})
			c.Abort()
			return
		}

		u, err := a.authenticateUserToken(token)
		if err != nil {
			c.JSON(401, gin.H{"error": "登录已过期，请重新登录"})
			c.Abort()
			return
		}
		c.Set("user", u)
		c.Next()
	}
}

func userTokenFromRequest(r *http.Request, allowQuery bool) string {
	token := strings.TrimSpace(r.Header.Get("X-User-Token"))
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("Authorization"))
	}
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("X-Auth-Token"))
	}
	if token == "" && allowQuery {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	return token
}

func (a *App) authenticateUserToken(token string) (*sharing.User, error) {
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	if a.cfg.SidecarEnabled() {
		identity, err := a.sidecar.Verify(token)
		if err != nil {
			return nil, err
		}
		return a.findOrCreateSidecarUser(identity), nil
	}

	users, _ := a.store.ReadUsers()
	for _, u := range users {
		if u.SessionToken == token {
			return u, nil
		}
	}
	return nil, fmt.Errorf("session not found")
}

func getUser(c *gin.Context) *sharing.User {
	u, exists := c.Get("user")
	if !exists {
		return nil
	}
	user, ok := u.(*sharing.User)
	if !ok {
		return nil
	}
	return user
}

// ── 用户认证 Handler ──────────────────────────────────

func (a *App) handleUserLogin(c *gin.Context) {
	if a.cfg.SidecarEnabled() {
		c.JSON(400, gin.H{"ok": false, "message": "当前已启用统一认证，请从统一登录入口进入"})
		return
	}

	var req struct {
		InviteCode string `json:"invite_code"`
		Name       string `json:"name"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"ok": false, "message": "无效的请求"})
		return
	}
	code := strings.TrimSpace(req.InviteCode)
	if code == "" {
		c.JSON(400, gin.H{"ok": false, "message": "请输入邀请码"})
		return
	}

	// 检查邀请码
	a.userOpMu.Lock()
	inv := a.store.GetInvite(code)
	if inv == nil {
		a.userOpMu.Unlock()
		c.JSON(403, gin.H{"ok": false, "message": "邀请码无效"})
		return
	}

	// 先查找已有用户（回头客直接登录）
	user := a.store.GetUserByInviteCode(code)
	if user == nil {
		// 首次使用 → 检查配额并创建用户
		if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
			a.userOpMu.Unlock()
			c.JSON(403, gin.H{"ok": false, "message": "邀请码已被使用"})
			return
		}
		settings := a.store.ReadSettings()
		user = &sharing.User{
			ID:          "user_" + shortID(),
			Name:        req.Name,
			InviteCode:  code,
			CreatedAt:   time.Now().Format("2006-01-02 15:04:05"),
			MaxLobsters: settings.DefaultMaxLobstersPerUser,
		}
		if user.Name == "" {
			user.Name = "龙虾爱好者"
		}
		inv.UsedCount++
		inv.UsedBy = append(inv.UsedBy, user.ID)
		a.store.SaveInvite(inv)
	}
	a.userOpMu.Unlock()

	// 生成 session token
	user.SessionToken = generateToken()
	user.LastLoginAt = time.Now().Format("2006-01-02 15:04:05")
	a.store.SaveUser(user)

	c.JSON(200, gin.H{
		"ok":    true,
		"token": user.SessionToken,
		"user": gin.H{
			"id": user.ID, "name": user.Name, "max_lobsters": user.MaxLobsters,
		},
	})
}

func (a *App) handleMe(c *gin.Context) {
	u := getUser(c)
	lobsters := a.store.GetLobstersByUser(u.ID)
	c.JSON(200, gin.H{
		"user": gin.H{
			"id":            u.ID,
			"name":          u.Name,
			"max_lobsters":  u.MaxLobsters,
			"created_at":    u.CreatedAt,
			"lobster_count": len(lobsters),
			"auth_source":   u.AuthSource,
			"organization":  u.CasdoorOrganization,
			"email":         u.Email,
		},
	})
}

func (a *App) handleListRegions(c *gin.Context) {
	regions := a.store.ListOnlineRegions()
	if regions == nil {
		regions = []string{}
	}
	c.JSON(200, gin.H{"regions": regions})
}

// ── 龙虾 Handler ──────────────────────────────────────

func (a *App) handleListMyLobsters(c *gin.Context) {
	u := getUser(c)
	lobsters := a.store.GetLobstersByUser(u.ID)
	if lobsters == nil {
		lobsters = []*sharing.Lobster{}
	}
	for _, l := range lobsters {
		l.EnsureMonthlyReset()
	}
	c.JSON(200, lobsters)
}

func (a *App) handleCreateLobster(c *gin.Context) {
	u := getUser(c)

	// 检查配额（加锁防止并发超配额）
	a.userOpMu.Lock()
	defer a.userOpMu.Unlock()
	count := a.store.CountLobstersByUser(u.ID)
	if count >= u.MaxLobsters {
		c.JSON(400, gin.H{"ok": false, "message": fmt.Sprintf("已达龙虾上限(%d只)，请联系虾主升级", u.MaxLobsters)})
		return
	}

	var req struct {
		Name   string `json:"name"`
		Region string `json:"region"` // 区域偏好，空=自动选最空闲
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"ok": false, "message": "无效的请求"})
		return
	}
	if req.Name == "" {
		req.Name = fmt.Sprintf("龙虾%d号", count+1)
	}

	// 选择节点（按区域偏好）
	node := a.store.SelectNode(req.Region)
	if node == nil && req.Region != "" {
		// 指定区域没有节点，尝试自动选
		node = a.store.SelectNode("")
	}
	if node == nil {
		c.JSON(503, gin.H{"ok": false, "message": "暂无可用节点，请联系虾主"})
		return
	}

	// 分配端口
	port, err := a.nodeSvc.AllocatePort(context.Background(), node)
	if err != nil {
		c.JSON(500, gin.H{"ok": false, "message": "端口分配失败: " + err.Error()})
		return
	}

	settings := a.store.ReadSettings()
	lobster := &sharing.Lobster{
		ID:                  "lobster_" + shortID(),
		UserID:              u.ID,
		Name:                req.Name,
		NodeID:              node.ID,
		NodeName:            node.Name,
		Region:              node.Region,
		Port:                port,
		Status:              "created",
		CreatedAt:           time.Now().Format("2006-01-02 15:04:05"),
		MonthlyTokenLimit:   settings.DefaultMonthlyTokenLimit,
		MonthlySpaceLimitMB: settings.DefaultMonthlySpaceLimitMB,
		QuotaResetMonth:     time.Now().Format("2006-01"),
	}

	// 创建远端实例目录
	if err := a.nodeSvc.CreateInstance(context.Background(), node, lobster.ID, port); err != nil {
		c.JSON(500, gin.H{"ok": false, "message": "创建实例失败: " + err.Error()})
		return
	}

	// 更新节点计数
	node.CurrentCount++
	a.store.SaveNode(node)
	a.store.SaveLobster(lobster)

	c.JSON(201, gin.H{"ok": true, "lobster": lobster})
}

func (a *App) handleGetLobster(c *gin.Context) {
	u := getUser(c)
	l := a.store.GetLobster(c.Param("id"))
	if l == nil || l.UserID != u.ID {
		c.JSON(404, gin.H{"error": "龙虾不存在"})
		return
	}
	l.EnsureMonthlyReset()
	c.JSON(200, l)
}

func (a *App) handleBindWeixin(c *gin.Context) {
	u := getUser(c)
	l := a.store.GetLobster(c.Param("id"))
	if l == nil || l.UserID != u.ID {
		c.JSON(404, gin.H{"error": "龙虾不存在"})
		return
	}

	node := a.store.GetNode(l.NodeID)
	if node == nil {
		c.JSON(500, gin.H{"error": "节点不存在"})
		return
	}

	// SSE 流式响应
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, _ := c.Writer.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	ctx := c.Request.Context()

	l.Status = "binding"
	a.store.SaveLobster(l)

	writeSSE := func(event, stage, message string) {
		data, _ := json.Marshal(map[string]string{"stage": stage, "message": message})
		fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, string(data))
		if flusher != nil {
			flusher.Flush()
		}
	}
	writeSSEData := func(event string, data string) {
		fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	writeSSE("progress", "start", "正在连接远端节点...")

	// SSH 流式执行 picoclaw auth weixin
	outCh, errCh := a.nodeSvc.BindWeixin(ctx, node, c.Param("id"))

	var qrSent bool
	var loginSuccess bool

	for line := range outCh {
		cleanLine := stripAnsi(strings.TrimSpace(line))
		if cleanLine == "" {
			continue
		}

		// 检测 QR Code Link（picoclaw 输出格式）
		if !qrSent && strings.Contains(cleanLine, "QR Code Link:") {
			// 提取 URL
			parts := strings.SplitN(cleanLine, "QR Code Link:", 2)
			if len(parts) == 2 {
				qrURL := strings.TrimSpace(parts[1])
				if strings.HasPrefix(qrURL, "http") {
					// 通过 JSON 序列化保证转义正确
					data, _ := json.Marshal(map[string]string{
						"stage":   "qrcode",
						"message": "请用微信扫描二维码",
						"url":     qrURL,
					})
					writeSSEData("qrcode", string(data))
					qrSent = true
					writeSSE("progress", "waiting", "等待扫码确认...")
					continue
				}
			}
		}

		// 检测登录成功
		if strings.Contains(cleanLine, "Login successful") || strings.Contains(cleanLine, "successfully") ||
			strings.Contains(cleanLine, "Saved") || strings.Contains(cleanLine, "saved") ||
			strings.Contains(cleanLine, "✓") || strings.Contains(cleanLine, "成功") {
			loginSuccess = true
		}

		// 过滤掉 logo 和二维码字符画，把有意义的进度推给前端
		if !strings.ContainsAny(cleanLine, "█▄▀▐▌") &&
			!strings.Contains(cleanLine, "Waiting for scan") &&
			len(cleanLine) < 200 {
			writeSSE("progress", "login", cleanLine)
		}
	}

	select {
	case err := <-errCh:
		if err != nil && !loginSuccess {
			writeSSE("error", "error", "微信绑定失败: "+err.Error())
			l.Status = "error"
			a.store.SaveLobster(l)
			return
		}
	default:
	}

	if loginSuccess {
		l.Status = "running"
		l.WeixinBound = true
		l.BoundAt = time.Now().Format("2006-01-02 15:04:05")
		a.store.SaveLobster(l)

		// 重启实例
		writeSSE("progress", "restart", "正在重启 picoclaw gateway...")
		_ = a.nodeSvc.RestartInstance(ctx, node, l)

		writeSSEData("done", `{"stage":"done","message":"微信绑定成功！龙虾已上线 🦞"}`)
	} else {
		writeSSE("error", "error", "未检测到登录成功，请重试")
		l.Status = "error"
		a.store.SaveLobster(l)
	}
}

func (a *App) handleStopLobster(c *gin.Context) {
	u := getUser(c)
	l := a.store.GetLobster(c.Param("id"))
	if l == nil || l.UserID != u.ID {
		c.JSON(404, gin.H{"error": "龙虾不存在"})
		return
	}
	node := a.store.GetNode(l.NodeID)
	if node != nil {
		_ = a.nodeSvc.StopInstance(context.Background(), node, l.ID)
	}
	l.Status = "stopped"
	a.store.SaveLobster(l)
	c.JSON(200, gin.H{"ok": true, "message": "龙虾已休息 💤"})
}

func (a *App) handleStartLobster(c *gin.Context) {
	u := getUser(c)
	l := a.store.GetLobster(c.Param("id"))
	if l == nil || l.UserID != u.ID {
		c.JSON(404, gin.H{"error": "龙虾不存在"})
		return
	}
	node := a.store.GetNode(l.NodeID)
	if node != nil {
		if err := a.nodeSvc.StartInstance(c.Request.Context(), node, l.ID, l.Port); err != nil {
			c.JSON(500, gin.H{"ok": false, "message": "启动实例失败: " + err.Error()})
			return
		}
	}
	l.Status = "running"
	a.store.SaveLobster(l)
	c.JSON(200, gin.H{"ok": true, "message": "龙虾已唤醒 🦞"})
}

func (a *App) handleDeleteLobster(c *gin.Context) {
	u := getUser(c)
	l := a.store.GetLobster(c.Param("id"))
	if l == nil || l.UserID != u.ID {
		c.JSON(404, gin.H{"error": "龙虾不存在"})
		return
	}
	node := a.store.GetNode(l.NodeID)
	if node != nil {
		_ = a.nodeSvc.RemoveInstance(context.Background(), node, l.ID, l.Port)
		a.userOpMu.Lock()
		node.CurrentCount--
		if node.CurrentCount < 0 {
			node.CurrentCount = 0
		}
		a.store.SaveNode(node)
		a.userOpMu.Unlock()
	}
	a.store.DeleteLobster(l.ID)
	c.JSON(200, gin.H{"ok": true, "message": "龙虾已释放 🌊"})
}

// ── 管理员 Handler ────────────────────────────────────

func (a *App) handleAdminLogin(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"ok": false, "message": "无效的请求"})
		return
	}
	token, ok := a.auth.Login(req.Password)
	if !ok {
		c.JSON(403, gin.H{"ok": false, "message": "密码错误"})
		return
	}
	a.auth.StoreToken(token)
	c.JSON(200, gin.H{"ok": true, "token": token})
}

// handleAdminGate 验证管理后台隐藏路径
// 前端管理页加载时调用此接口，确认当前访问的 URL 路径匹配 AdminPath 配置
// 如果匹配则返回 200，不匹配返回 404（前端显示 404 页面）
func (a *App) handleAdminGate(c *gin.Context) {
	pathKey := c.Query("path")
	if pathKey == "" || a.cfg.AdminPath == "" {
		c.JSON(404, gin.H{"ok": false, "error": "not found"})
		return
	}
	// 对比：去掉前导 / 后比较
	expected := strings.TrimPrefix(a.cfg.AdminPath, "/")
	given := strings.TrimPrefix(pathKey, "/")
	if given != expected {
		c.JSON(404, gin.H{"ok": false, "error": "not found"})
		return
	}
	c.JSON(200, gin.H{
		"ok":                true,
		"requires_password": a.cfg.AdminPassword != "",
	})
}

func (a *App) handleAdminOverview(c *gin.Context) {
	users, _ := a.store.ReadUsers()
	lobsters, _ := a.store.ReadLobsters()
	nodes, _ := a.store.ReadNodes()

	running := 0
	for _, l := range lobsters {
		if l.Status == "running" {
			running++
		}
	}

	c.JSON(200, gin.H{
		"total_users":      len(users),
		"total_lobsters":   len(lobsters),
		"running_lobsters": running,
		"total_nodes":      len(nodes),
	})
}

func (a *App) handleAdminLobsters(c *gin.Context) {
	all, _ := a.store.ReadLobsters()
	list := make([]*sharing.Lobster, 0, len(all))
	for _, l := range all {
		list = append(list, l)
	}
	c.JSON(200, list)
}

func (a *App) handleListNodes(c *gin.Context) {
	all, _ := a.store.ReadNodes()
	list := make([]*sharing.Node, 0, len(all))
	for _, n := range all {
		list = append(list, n)
	}
	c.JSON(200, list)
}

func (a *App) handleAddNode(c *gin.Context) {
	var req sharing.Node
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"ok": false, "message": "无效的 JSON"})
		return
	}
	req.ID = "node_" + shortID()
	req.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	if req.Type == "" {
		req.Type = "ssh"
	}
	if req.Status == "" {
		req.Status = "offline"
	}
	if req.Type == "local" {
		if strings.TrimSpace(req.Host) == "" {
			req.Host = "local"
		}
		req.SSHPort = 0
		req.SSHUser = ""
		req.SSHPassword = ""
		if strings.TrimSpace(req.RemoteHome) == "" {
			req.RemoteHome = filepath.Join(a.cfg.ShareClawHome, "local-nodes", req.ID)
		}
	} else {
		if req.SSHPort <= 0 {
			req.SSHPort = 22
		}
		if req.SSHUser == "" {
			req.SSHUser = "root"
		}
		if req.RemoteHome == "" {
			req.RemoteHome = "/opt/pinhaoclaw"
		}
	}
	if req.MaxLobsters <= 0 {
		req.MaxLobsters = 10
	}
	if req.Region == "" {
		req.Region = "未设置"
	}
	a.store.SaveNode(&req)
	c.JSON(201, gin.H{"ok": true, "node": req})
}

func (a *App) handleDeleteNode(c *gin.Context) {
	a.store.DeleteNode(c.Param("id"))
	c.JSON(200, gin.H{"ok": true})
}

func (a *App) handleDeployNode(c *gin.Context) {
	node := a.store.GetNode(c.Param("id"))
	if node == nil {
		c.JSON(404, gin.H{"error": "节点不存在"})
		return
	}
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, _ := c.Writer.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	ch := make(chan claw.SSEEvent, 50)
	go a.nodeSvc.Deploy(c.Request.Context(), node, ch)
	for event := range ch {
		fmt.Fprintf(c.Writer, "%s", event.ToSSEFormat())
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func (a *App) handleTestNode(c *gin.Context) {
	node := a.store.GetNode(c.Param("id"))
	if node == nil {
		c.JSON(404, gin.H{"error": "节点不存在"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.nodeSvc.TestConnection(ctx, node); err != nil {
		c.JSON(200, gin.H{"ok": false, "message": "连接失败: " + err.Error()})
		return
	}
	env, _ := a.nodeSvc.DetectEnvironment(ctx, node)
	node.Status = "online"
	a.store.SaveNode(node)
	c.JSON(200, gin.H{"ok": true, "message": "连接成功", "env": env})
}

func (a *App) handleListInvites(c *gin.Context) {
	all, _ := a.store.ReadInvites()
	c.JSON(200, all)
}

func (a *App) handleCreateInvite(c *gin.Context) {
	var req struct {
		CreatedBy string `json:"created_by"`
		MaxUses   int    `json:"max_uses"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"ok": false, "message": "无效的请求"})
		return
	}
	if req.CreatedBy == "" {
		req.CreatedBy = "虾主"
	}
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	code := uuid.New().String()[:8]
	inv := &sharing.Invite{
		Code:      code,
		CreatedBy: req.CreatedBy,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		MaxUses:   req.MaxUses,
	}
	a.store.SaveInvite(inv)

	host := c.Request.Host
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	c.JSON(201, gin.H{
		"ok":   true,
		"code": code,
		"url":  fmt.Sprintf("%s://%s/?code=%s", scheme, host, code),
	})
}

func (a *App) handleDeleteInvite(c *gin.Context) {
	a.store.DeleteInvite(c.Param("code"))
	c.JSON(200, gin.H{"ok": true})
}

func (a *App) handleGetSettings(c *gin.Context) {
	c.JSON(200, a.store.ReadSettings())
}

func (a *App) handleUpdateSettings(c *gin.Context) {
	var updates map[string]any
	if err := c.BindJSON(&updates); err != nil {
		c.JSON(400, gin.H{"ok": false, "message": "无效的请求"})
		return
	}
	s := a.store.ReadSettings()
	if v, ok := updates["default_monthly_token_limit"]; ok {
		if f, ok := v.(float64); ok {
			s.DefaultMonthlyTokenLimit = int64(f)
		}
	}
	if v, ok := updates["default_monthly_space_limit_mb"]; ok {
		if f, ok := v.(float64); ok {
			s.DefaultMonthlySpaceLimitMB = int64(f)
		}
	}
	if v, ok := updates["default_max_lobsters_per_user"]; ok {
		if f, ok := v.(float64); ok {
			s.DefaultMaxLobstersPerUser = int(f)
		}
	}
	a.store.WriteSettings(s)
	c.JSON(200, s)
}

// ── Skill 库 Handler（用户浏览） ──────────────────────

func (a *App) handleListSkills(c *gin.Context) {
	all, _ := a.store.ReadSkillRegistry()
	list := make([]*sharing.SkillRegistryEntry, 0, len(all))
	for _, s := range all {
		list = append(list, s)
	}
	c.JSON(200, gin.H{"skills": list})
}

func (a *App) handleGetSkill(c *gin.Context) {
	entry := a.store.GetSkillRegistryEntry(c.Param("slug"))
	if entry == nil {
		c.JSON(404, gin.H{"error": "Skill 不存在"})
		return
	}
	c.JSON(200, entry)
}

// ── 龙虾 Skill 管理 Handler ──────────────────────────

func (a *App) handleListLobsterSkills(c *gin.Context) {
	u := getUser(c)
	l := a.store.GetLobster(c.Param("id"))
	if l == nil || l.UserID != u.ID {
		c.JSON(404, gin.H{"error": "龙虾不存在"})
		return
	}
	installed, _ := a.store.ReadLobsterSkills(l.ID)
	node := a.store.GetNode(l.NodeID)
	var remoteSlugs []string
	if node != nil {
		slugs, err := a.nodeSvc.ListInstalledSkills(context.Background(), node, l.ID)
		if err == nil {
			remoteSlugs = slugs
		}
	}
	registry, _ := a.store.ReadSkillRegistry()
	type SkillInfo struct {
		Slug        string `json:"slug"`
		DisplayName string `json:"display_name"`
		Summary     string `json:"summary"`
		Icon        string `json:"icon"`
		Version     string `json:"version"`
		InstalledAt string `json:"installed_at,omitempty"`
	}
	var result []SkillInfo
	seen := make(map[string]bool)
	for _, slug := range remoteSlugs {
		info := SkillInfo{Slug: slug}
		if entry, ok := registry[slug]; ok {
			info.DisplayName = entry.DisplayName
			info.Summary = entry.Summary
			info.Icon = entry.Icon
			info.Version = entry.Version
		} else {
			info.DisplayName = slug
		}
		for _, ls := range installed {
			if ls.Slug == slug {
				info.InstalledAt = ls.InstalledAt
				break
			}
		}
		seen[slug] = true
		result = append(result, info)
	}
	for _, ls := range installed {
		if !seen[ls.Slug] {
			info := SkillInfo{Slug: ls.Slug, InstalledAt: ls.InstalledAt, Version: ls.Version}
			if entry, ok := registry[ls.Slug]; ok {
				info.DisplayName = entry.DisplayName
				info.Summary = entry.Summary
				info.Icon = entry.Icon
			} else {
				info.DisplayName = ls.Slug
			}
			result = append(result, info)
		}
	}
	if result == nil {
		result = []SkillInfo{}
	}
	c.JSON(200, gin.H{"skills": result})
}

func (a *App) handleInstallSkill(c *gin.Context) {
	u := getUser(c)
	l := a.store.GetLobster(c.Param("id"))
	if l == nil || l.UserID != u.ID {
		c.JSON(404, gin.H{"error": "龙虾不存在"})
		return
	}
	var req struct {
		Slug string `json:"slug"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"ok": false, "message": "无效的请求"})
		return
	}
	if req.Slug == "" {
		c.JSON(400, gin.H{"ok": false, "message": "slug 必填"})
		return
	}
	skill := a.store.GetSkillRegistryEntry(req.Slug)
	if skill == nil {
		c.JSON(404, gin.H{"ok": false, "message": "Skill 库中不存在该 Skill"})
		return
	}
	node := a.store.GetNode(l.NodeID)
	if node == nil {
		c.JSON(500, gin.H{"ok": false, "message": "节点不存在"})
		return
	}
	if err := a.nodeSvc.InstallSkill(context.Background(), node, l.ID, skill); err != nil {
		c.JSON(500, gin.H{"ok": false, "message": "安装 Skill 失败: " + err.Error()})
		return
	}
	a.store.SaveLobsterSkill(l.ID, &sharing.LobsterSkill{
		Slug:        skill.Slug,
		Version:     skill.Version,
		InstalledAt: time.Now().Format("2006-01-02 15:04:05"),
	})
	c.JSON(200, gin.H{"ok": true, "message": fmt.Sprintf("Skill %s 安装成功 ✨", skill.DisplayName)})
}

func (a *App) handleUninstallSkill(c *gin.Context) {
	u := getUser(c)
	l := a.store.GetLobster(c.Param("id"))
	if l == nil || l.UserID != u.ID {
		c.JSON(404, gin.H{"error": "龙虾不存在"})
		return
	}
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(400, gin.H{"ok": false, "message": "slug 必填"})
		return
	}
	node := a.store.GetNode(l.NodeID)
	if node == nil {
		c.JSON(500, gin.H{"ok": false, "message": "节点不存在"})
		return
	}
	if err := a.nodeSvc.UninstallSkill(context.Background(), node, l.ID, slug); err != nil {
		c.JSON(500, gin.H{"ok": false, "message": "卸载 Skill 失败: " + err.Error()})
		return
	}
	a.store.RemoveLobsterSkill(l.ID, slug)
	c.JSON(200, gin.H{"ok": true, "message": fmt.Sprintf("Skill %s 已卸载", slug)})
}

// ── 管理员 Skill 库管理 Handler ──────────────────────

func (a *App) handleAdminListSkills(c *gin.Context) {
	all, _ := a.store.ReadSkillRegistry()
	list := make([]*sharing.SkillRegistryEntry, 0, len(all))
	for _, s := range all {
		list = append(list, s)
	}
	c.JSON(200, gin.H{"skills": list})
}

func (a *App) handleAdminCreateSkill(c *gin.Context) {
	var entry sharing.SkillRegistryEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(400, gin.H{"ok": false, "message": "无效的 JSON"})
		return
	}
	if entry.Slug == "" {
		c.JSON(400, gin.H{"ok": false, "message": "slug 必填"})
		return
	}
	if entry.DisplayName == "" {
		entry.DisplayName = entry.Slug
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	entry.CreatedAt = now
	entry.UpdatedAt = now
	a.store.SaveSkillRegistryEntry(&entry)
	c.JSON(201, gin.H{"ok": true, "skill": entry})
}

func (a *App) handleAdminUpdateSkill(c *gin.Context) {
	slug := c.Param("slug")
	existing := a.store.GetSkillRegistryEntry(slug)
	if existing == nil {
		c.JSON(404, gin.H{"ok": false, "message": "Skill 不存在"})
		return
	}
	var updates sharing.SkillRegistryEntry
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(400, gin.H{"ok": false, "message": "无效的 JSON"})
		return
	}
	updates.Slug = slug
	updates.CreatedAt = existing.CreatedAt
	updates.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
	a.store.SaveSkillRegistryEntry(&updates)
	c.JSON(200, gin.H{"ok": true, "skill": updates})
}

func (a *App) handleAdminDeleteSkill(c *gin.Context) {
	slug := c.Param("slug")
	a.store.DeleteSkillRegistryEntry(slug)
	c.JSON(200, gin.H{"ok": true})
}

// ── 辅助函数 ──────────────────────────────────────────

func shortID() string { return uuid.New().String()[:8] }

func generateToken() string {
	b := make([]byte, 44)
	rand.Read(b)
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 44)
	for i := range result {
		result[i] = chars[int(b[i])%len(chars)]
	}
	return string(result)
}

func stripAnsi(s string) string {
	// 简单去除 ANSI 转义序列
	result := strings.Builder{}
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func shouldBlockFrontendFallback(requestPath string) bool {
	normalized := strings.ToLower(strings.TrimSpace(requestPath))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "..") {
		return true
	}
	sensitivePrefixes := []string{
		"/scripts",
		"/.git",
		"/.env",
		"/config",
		"/server",
		"/claw",
		"/sharing",
	}
	for _, prefix := range sensitivePrefixes {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+"/") {
			return true
		}
	}
	sensitiveExts := map[string]struct{}{
		".sh":   {},
		".env":  {},
		".go":   {},
		".mod":  {},
		".sum":  {},
		".pem":  {},
		".key":  {},
		".md":   {},
		".yaml": {},
		".yml":  {},
	}
	_, blocked := sensitiveExts[path.Ext(normalized)]
	return blocked
}

func (a *App) handleQRCode(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.JSON(400, gin.H{"error": "url required"})
		return
	}
	png, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.Data(200, "image/png", png)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		// Allow same-origin and common development origins
		allowedOrigins := map[string]bool{
			"http://localhost:5173": true,
			"http://localhost:3000": true,
			"http://127.0.0.1:5173": true,
		}
		if origin != "" && (allowedOrigins[origin] || strings.HasPrefix(origin, c.Request.Host)) {
			c.Header("Access-Control-Allow-Origin", origin)
		}
		c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token, X-User-Token, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
