package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	HandshakeTimeout: 0,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源（CORS 已在 corsMiddleware 处理）
	},
}

// handleBindWeixinWS WebSocket 版微信绑定（给小程序用）
// 和 SSE 版共享同一套 BindWeixin 逻辑，消息格式保持一致
func (a *App) handleBindWeixinWS(c *gin.Context) {
	// 手动验证 token（WebSocket 握手无法设置自定义 Header）
	token := userTokenFromRequest(c.Request, true)
	if token == "" {
		c.JSON(401, gin.H{"error": "请先登录"})
		return
	}
	currentUser, err := a.authenticateUserToken(token)
	if err != nil {
		c.JSON(401, gin.H{"error": "登录已过期"})
		return
	}

	lobsterID := c.Param("id")
	l := a.store.GetLobster(lobsterID)
	if l == nil {
		c.JSON(404, gin.H{"error": "龙虾不存在"})
		return
	}

	// 校验龙虾所有权
	if l.UserID != currentUser.ID {
		c.JSON(403, gin.H{"error": "无权操作此龙虾"})
		return
	}

	node := a.store.GetNode(l.NodeID)
	if node == nil {
		c.JSON(500, gin.H{"error": "节点不存在"})
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	send := func(event, stage, message string) {
		data, _ := json.Marshal(map[string]string{
			"event":   event,
			"stage":   stage,
			"message": message,
		})
		conn.WriteMessage(websocket.TextMessage, data)
	}
	sendData := func(event string, payload map[string]string) {
		payload["event"] = event
		data, _ := json.Marshal(payload)
		conn.WriteMessage(websocket.TextMessage, data)
	}

	l.Status = "binding"
	a.store.SaveLobster(l)

	send("progress", "start", "正在连接远端节点...")

	ctx := c.Request.Context()
	outCh, errCh := a.nodeSvc.BindWeixin(ctx, node, lobsterID)

	var qrSent, loginSuccess bool

	for line := range outCh {
		cleanLine := stripAnsi(strings.TrimSpace(line))
		if cleanLine == "" {
			continue
		}

		if !qrSent && strings.Contains(cleanLine, "QR Code Link:") {
			parts := strings.SplitN(cleanLine, "QR Code Link:", 2)
			if len(parts) == 2 {
				qrURL := strings.TrimSpace(parts[1])
				if strings.HasPrefix(qrURL, "http") {
					sendData("qrcode", map[string]string{
						"stage":   "qrcode",
						"message": "请用微信扫描二维码",
						"url":     qrURL,
					})
					qrSent = true
					send("progress", "waiting", "等待扫码确认...")
					continue
				}
			}
		}

		if strings.Contains(cleanLine, "Login successful") || strings.Contains(cleanLine, "successfully") ||
			strings.Contains(cleanLine, "Saved") || strings.Contains(cleanLine, "saved") ||
			strings.Contains(cleanLine, "✓") || strings.Contains(cleanLine, "成功") {
			loginSuccess = true
		}

		if !strings.ContainsAny(cleanLine, "█▄▀▐▌") &&
			!strings.Contains(cleanLine, "Waiting for scan") &&
			len(cleanLine) < 200 {
			send("progress", "login", cleanLine)
		}
	}

	select {
	case err := <-errCh:
		if err != nil && !loginSuccess {
			send("error", "error", "微信绑定失败: "+err.Error())
			l.Status = "error"
			a.store.SaveLobster(l)
			return
		}
	default:
	}

	if loginSuccess {
		l.Status = "running"
		l.WeixinBound = true
		l.BoundAt = fmt.Sprintf("%s", time.Now().Format("2006-01-02 15:04:05"))
		a.store.SaveLobster(l)

		send("progress", "restart", "正在重启 picoclaw gateway...")
		_ = a.nodeSvc.RestartInstance(ctx, node, l)

		sendData("done", map[string]string{
			"stage":   "done",
			"message": "微信绑定成功！龙虾已上线 🦞",
		})
	} else {
		send("error", "error", "未检测到登录成功，请重试")
		l.Status = "error"
		a.store.SaveLobster(l)
	}
}
