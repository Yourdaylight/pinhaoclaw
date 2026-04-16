package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pinhaoclaw/pinhaoclaw/config"
	"github.com/pinhaoclaw/pinhaoclaw/sharing"
)

// SidecarIdentity represents the user identity returned by the auth sidecar.
type SidecarIdentity struct {
	Sub          string `json:"sub"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Avatar       string `json:"avatar"`
	Organization string `json:"organization"`
}

type sidecarVerifyResponse struct {
	Ok       bool             `json:"ok"`
	Token    string           `json:"token"`
	Identity *SidecarIdentity `json:"identity"`
	Error    string           `json:"error,omitempty"`
}

// SidecarClient calls the casdoor-auth-sidecar verify endpoint.
type SidecarClient struct {
	baseURL     string
	httpClient  *http.Client
	proxyClient *http.Client // for proxying (no redirect auto-follow)
}

func NewSidecarClient(cfg *config.Config) *SidecarClient {
	return &SidecarClient{
		baseURL: cfg.AuthSidecarURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		proxyClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // don't follow redirects
			},
		},
	}
}

// Verify calls POST /api/auth/verify on the sidecar and returns the user identity.
func (s *SidecarClient) Verify(token string) (*SidecarIdentity, error) {
	if s.baseURL == "" {
		return nil, fmt.Errorf("auth sidecar URL not configured")
	}
	payload, _ := json.Marshal(map[string]string{"token": token})
	req, err := http.NewRequest(http.MethodPost, s.baseURL+"/api/auth/verify", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("sidecar request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sidecar unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sidecar verify failed (%d): %s", resp.StatusCode, string(body))
	}

	var result sidecarVerifyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("sidecar response parse error: %w", err)
	}
	if !result.Ok || result.Identity == nil {
		return nil, fmt.Errorf("sidecar verify rejected: %s", result.Error)
	}
	return result.Identity, nil
}

// findOrCreateSidecarUser maps a sidecar identity to a local user.
func (a *App) findOrCreateSidecarUser(identity *SidecarIdentity) *sharing.User {
	user := a.store.GetUserByCasdoorSub(identity.Sub)
	settings := a.store.ReadSettings()
	if user == nil {
		user = &sharing.User{
			ID:          "user_" + shortID(),
			CreatedAt:   time.Now().Format("2006-01-02 15:04:05"),
			MaxLobsters: settings.DefaultMaxLobstersPerUser,
			AuthSource:  "sidecar",
			CasdoorSub:  identity.Sub,
		}
	}
	if user.MaxLobsters <= 0 {
		user.MaxLobsters = settings.DefaultMaxLobstersPerUser
	}
	user.AuthSource = "sidecar"
	user.Name = firstNonEmpty(identity.DisplayName, user.Name, identity.Username, identity.Email, "龙虾用户")
	user.CasdoorSub = identity.Sub
	user.CasdoorUsername = identity.Username
	user.CasdoorOrganization = identity.Organization
	user.Email = identity.Email
	user.Avatar = identity.Avatar
	user.LastLoginAt = time.Now().Format("2006-01-02 15:04:05")
	a.store.SaveUser(user)
	return user
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// handleAuthConfig returns auth mode info for the frontend.
func (a *App) handleAuthConfig(c *gin.Context) {
	if a.cfg.SidecarEnabled() {
		result := gin.H{
			"mode":            "sidecar",
			"sidecar_enabled": true,
			"login_url":       a.cfg.PublicOrigin + "/api/auth/sidecar/login",
		}
		if a.cfg.CasdoorEndpoint != "" {
			result["casdoor_logout_url"] = a.cfg.PublicOrigin + "/api/auth/sidecar/logout"
		}
		c.JSON(http.StatusOK, result)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"mode":            "invite",
		"sidecar_enabled": false,
	})
}

// handleSidecarLogin proxies GET /api/auth/sidecar/login to sidecar's /api/auth/login.
// Sidecar generates an OAuth2 state and 302 redirects to Casdoor.
// We pass that redirect through to the browser.
func (a *App) handleSidecarLogin(c *gin.Context) {
	sidecarURL := a.sidecar.baseURL + "/api/auth/login"
	// Forward prompt parameter (e.g. prompt=login forces re-authentication)
	if prompt := c.Query("prompt"); prompt != "" {
		sidecarURL += "?prompt=" + prompt
	}
	req, err := http.NewRequest(http.MethodGet, sidecarURL, nil)
	if err != nil {
		c.String(http.StatusInternalServerError, "sidecar login request error")
		return
	}

	resp, err := a.sidecar.proxyClient.Do(req)
	if err != nil {
		c.String(http.StatusBadGateway, "sidecar unreachable")
		return
	}
	defer resp.Body.Close()

	// Sidecar returns 302 to Casdoor — pass the redirect through
	if loc := resp.Header.Get("Location"); loc != "" {
		c.Redirect(resp.StatusCode, loc)
		return
	}

	// Non-redirect response: forward as-is
	for k, vs := range resp.Header {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

// handleSidecarLogoutPage proxies to sidecar's browser-facing logout relay page.
// The relay clears local session data, then completes Casdoor logout in a
// hidden iframe so the main window never leaves the app domain.
func (a *App) handleSidecarLogoutPage(c *gin.Context) {
	c.Redirect(http.StatusFound, strings.TrimRight(a.cfg.PublicOrigin, "/")+"/")
}

func (a *App) handleSidecarLogout(c *gin.Context) {
	token := userTokenFromRequest(c.Request, false)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "请先登录"})
		return
	}

	payload, _ := json.Marshal(map[string]string{"token": token})
	req, err := http.NewRequest(http.MethodPost, a.sidecar.baseURL+"/api/auth/logout", bytes.NewReader(payload))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "message": "sidecar logout request error"})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Token", token)

	resp, err := a.sidecar.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "message": "sidecar unreachable"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "message": strings.TrimSpace(string(body))})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleSidecarLogoutComplete proxies the iframe callback page that signals the
// main logout relay after Casdoor has finished clearing its own session.
func (a *App) handleSidecarLogoutComplete(c *gin.Context) {
	sidecarURL := a.sidecar.baseURL + "/api/auth/logout-complete"
	req, err := http.NewRequest(http.MethodGet, sidecarURL, nil)
	if err != nil {
		c.Redirect(http.StatusFound, strings.TrimRight(a.cfg.PublicOrigin, "/")+"/")
		return
	}
	req.URL.RawQuery = c.Request.URL.RawQuery

	resp, err := a.sidecar.proxyClient.Do(req)
	if err != nil {
		c.Redirect(http.StatusFound, strings.TrimRight(a.cfg.PublicOrigin, "/")+"/")
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		c.Redirect(resp.StatusCode, loc)
		return
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}

// handleSidecarCallback proxies GET /api/auth/sidecar/callback to sidecar's /api/auth/callback.
// Casdoor redirects the browser here with code+state. We forward to sidecar,
// which exchanges the code, creates a session, and returns an HTML bridge page
// that writes the token to localStorage and posts a message to the opener window.
func (a *App) handleSidecarCallback(c *gin.Context) {
	sidecarURL := a.sidecar.baseURL + "/api/auth/callback"
	req, err := http.NewRequest(http.MethodGet, sidecarURL, nil)
	if err != nil {
		c.String(http.StatusInternalServerError, "sidecar callback request error")
		return
	}
	// Forward query params (code, state)
	req.URL.RawQuery = c.Request.URL.RawQuery

	resp, err := a.sidecar.proxyClient.Do(req)
	if err != nil {
		c.String(http.StatusBadGateway, "sidecar unreachable")
		return
	}
	defer resp.Body.Close()

	// Forward all headers
	for k, vs := range resp.Header {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}
