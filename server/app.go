package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/garden/pinhaoclaw/claw"
	"github.com/garden/pinhaoclaw/config"
	"github.com/garden/pinhaoclaw/sharing"
)

type App struct {
	cfg     *config.Config
	store   *sharing.Store
	nodeSvc *claw.NodeService
	auth    *AdminAuth
	router  *gin.Engine
}

func NewApp(cfg *config.Config) *App {
	store := sharing.NewStore(cfg.ShareClawHome)
	app := &App{
		cfg:     cfg,
		store:   store,
		nodeSvc: claw.NewNodeService(store),
		auth:    NewAdminAuth(cfg.AdminPassword),
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
	api.POST("/auth/login", a.handleUserLogin)
	api.GET("/auth/me", a.requireUser(), a.handleMe)
	api.GET("/regions", a.requireUser(), a.handleListRegions)
	api.GET("/qrcode", a.handleQRCode)

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

	// ── 管理后台隐藏入口验证（无需登录）──
	// 前端管理页加载时调用，确认当前访问路径匹配 AdminPath 配置
	r.GET("/api/admin/gate", a.handleAdminGate)

	// ── 管理后台隐藏路径路由（AdminPath 如 /mgr-x7Kp9qZ）──
	// 注意：此段必须在 frontendDir 定义之后（见下方静态文件区域）
	// 此处仅注册路由占位，实际逻辑在下方 frontendDir 声明后处理

	// ── 静态文件（H5 前端产物）──
	// uni-app H5 构建产物放在 pinhaoclaw-frontend/dist/build/h5
	const frontendDir = "pinhaoclaw-frontend/dist/build/h5"
	r.Static("/assets", frontendDir+"/assets")
	r.StaticFile("/favicon.ico", frontendDir+"/favicon.ico")

	// 管理后台隐藏路径（在此处定义，因为需要 frontendDir）
	if a.cfg.AdminPath != "" {
		adminPath := strings.TrimPrefix(a.cfg.AdminPath, "/")
		r.GET("/"+adminPath, func(c *gin.Context) {
			c.File(frontendDir + "/index.html")
		})
		r.GET("/"+adminPath+"/*filepath", func(c *gin.Context) {
			c.File(frontendDir + "/index.html")
		})
	}

	r.NoRoute(func(c *gin.Context) {
		// 所有未匹配路由返回 index.html（SPA 路由支持）
		if !strings.HasPrefix(c.Request.URL.Path, "/api") &&
			!strings.HasPrefix(c.Request.URL.Path, "/ws") {
			c.File(frontendDir + "/index.html")
			return
		}
		c.JSON(404, gin.H{"error": "not found"})
	})

	a.router = r
}

// ── 用户认证中间件 ──────────────────────────────────

func (a *App) requireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-User-Token")
		if token == "" {
			token = c.Query("token")
		}
		if token == "" {
			c.JSON(401, gin.H{"error": "请先登录"})
			c.Abort()
			return
		}
		users, _ := a.store.ReadUsers()
		for _, u := range users {
			if u.SessionToken == token {
				c.Set("user", u)
				c.Next()
				return
			}
		}
		c.JSON(401, gin.H{"error": "登录已过期，请重新输入邀请码"})
		c.Abort()
	}
}

func getUser(c *gin.Context) *sharing.User {
	u, _ := c.Get("user")
	return u.(*sharing.User)
}

// ── 用户认证 Handler ──────────────────────────────────

func (a *App) handleUserLogin(c *gin.Context) {
	var req struct {
		InviteCode string `json:"invite_code"`
		Name       string `json:"name"`
	}
	c.BindJSON(&req)
	code := strings.TrimSpace(req.InviteCode)
	if code == "" {
		c.JSON(400, gin.H{"ok": false, "message": "请输入邀请码"})
		return
	}

	// 检查邀请码
	inv := a.store.GetInvite(code)
	if inv == nil {
		c.JSON(403, gin.H{"ok": false, "message": "邀请码无效"})
		return
	}

	// 先查找已有用户（回头客直接登录）
	user := a.store.GetUserByInviteCode(code)
	if user == nil {
		// 首次使用 → 检查配额并创建用户
		if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
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
			"id": u.ID, "name": u.Name, "max_lobsters": u.MaxLobsters,
			"created_at": u.CreatedAt, "lobster_count": len(lobsters),
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

	// 检查配额
	count := a.store.CountLobstersByUser(u.ID)
	if count >= u.MaxLobsters {
		c.JSON(400, gin.H{"ok": false, "message": fmt.Sprintf("已达龙虾上限(%d只)，请联系虾主升级", u.MaxLobsters)})
		return
	}

	var req struct {
		Name   string `json:"name"`
		Region string `json:"region"` // 区域偏好，空=自动选最空闲
	}
	c.BindJSON(&req)
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
		fmt.Fprintf(c.Writer, "event: %s\ndata: {\"stage\":\"%s\",\"message\":\"%s\"}\n\n", event, stage, message)
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
	outCh, errCh := a.nodeSvc.BindWeixin(ctx, node)

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

		// 重启 gateway
		writeSSE("progress", "restart", "正在重启 picoclaw gateway...")
		_ = a.nodeSvc.RestartGateway(ctx, node)

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
	// TODO: 实际重启远端实例
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
		_ = a.nodeSvc.RemoveInstance(context.Background(), node, l.ID)
		node.CurrentCount--
		if node.CurrentCount < 0 {
			node.CurrentCount = 0
		}
		a.store.SaveNode(node)
	}
	a.store.DeleteLobster(l.ID)
	c.JSON(200, gin.H{"ok": true, "message": "龙虾已释放 🌊"})
}

// ── 管理员 Handler ────────────────────────────────────

func (a *App) handleAdminLogin(c *gin.Context) {
	var req struct{ Password string `json:"password"` }
	c.BindJSON(&req)
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
		"ok":       true,
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
		"total_users":     len(users),
		"total_lobsters":  len(lobsters),
		"running_lobsters": running,
		"total_nodes":     len(nodes),
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
	if req.Status == "" {
		req.Status = "offline"
	}
	if req.SSHPort <= 0 {
		req.SSHPort = 22
	}
	if req.SSHUser == "" {
		req.SSHUser = "root"
	}
	if req.RemoteHome == "" {
		req.RemoteHome = "/opt/shareclaw"
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
	c.BindJSON(&req)
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
	c.BindJSON(&updates)
	s := a.store.ReadSettings()
	if v, ok := updates["default_monthly_token_limit"]; ok {
		if f, ok := v.(float64); ok { s.DefaultMonthlyTokenLimit = int64(f) }
	}
	if v, ok := updates["default_monthly_space_limit_mb"]; ok {
		if f, ok := v.(float64); ok { s.DefaultMonthlySpaceLimitMB = int64(f) }
	}
	if v, ok := updates["default_max_lobsters_per_user"]; ok {
		if f, ok := v.(float64); ok { s.DefaultMaxLobstersPerUser = int(f) }
	}
	a.store.WriteSettings(s)
	c.JSON(200, s)
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
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token, X-User-Token")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
