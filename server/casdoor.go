package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/garden/pinhaoclaw/config"
	"github.com/garden/pinhaoclaw/sharing"
)

const oauthStateTTL = 10 * time.Minute

type CasdoorClient struct {
	cfg        *config.Config
	httpClient *http.Client
	mu         sync.Mutex
	states     map[string]time.Time
}

type casdoorTokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type casdoorClaims struct {
	Sub               string `json:"sub"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	DisplayName       string `json:"displayName"`
	Email             string `json:"email"`
	Picture           string `json:"picture"`
	Avatar            string `json:"avatar"`
	Owner             string `json:"owner"`
}

type CasdoorIdentity struct {
	Sub          string
	Username     string
	DisplayName  string
	Email        string
	Avatar       string
	Organization string
}

func NewCasdoorClient(cfg *config.Config) *CasdoorClient {
	return &CasdoorClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		states: make(map[string]time.Time),
	}
}

func (c *CasdoorClient) LoginURL() (string, error) {
	if !c.cfg.CasdoorEnabled() {
		return "", errors.New("casdoor is not enabled")
	}
	state := generateToken()
	c.storeState(state)
	query := url.Values{}
	query.Set("client_id", c.cfg.CasdoorClientID)
	query.Set("redirect_uri", c.cfg.CasdoorRedirectURL())
	query.Set("response_type", "code")
	query.Set("scope", "openid profile email")
	query.Set("state", state)
	return c.cfg.CasdoorEndpoint + "/login/oauth/authorize?" + query.Encode(), nil
}

func (c *CasdoorClient) storeState(state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanupLocked(time.Now())
	c.states[state] = time.Now().Add(oauthStateTTL)
}

func (c *CasdoorClient) VerifyState(state string) bool {
	if state == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	expireAt, ok := c.states[state]
	if !ok {
		c.cleanupLocked(time.Now())
		return false
	}
	delete(c.states, state)
	if time.Now().After(expireAt) {
		c.cleanupLocked(time.Now())
		return false
	}
	c.cleanupLocked(time.Now())
	return true
}

func (c *CasdoorClient) cleanupLocked(now time.Time) {
	for state, expireAt := range c.states {
		if now.After(expireAt) {
			delete(c.states, state)
		}
	}
}

func (c *CasdoorClient) ExchangeCode(code string) (*casdoorTokenResponse, error) {
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     c.cfg.CasdoorClientID,
		"client_secret": c.cfg.CasdoorClientSecret,
		"code":          code,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, c.cfg.CasdoorEndpoint+"/api/login/oauth/access_token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("casdoor token exchange failed: %s", strings.TrimSpace(string(respBody)))
	}

	result := &casdoorTokenResponse{}
	if err := json.Unmarshal(respBody, result); err != nil {
		return nil, err
	}
	if result.AccessToken == "" && result.IDToken == "" {
		return nil, fmt.Errorf("casdoor token response missing token")
	}
	return result, nil
}

func (c *CasdoorClient) ParseIdentity(tokenResp *casdoorTokenResponse) (*CasdoorIdentity, error) {
	rawToken := tokenResp.IDToken
	if rawToken == "" {
		rawToken = tokenResp.AccessToken
	}
	claims := &casdoorClaims{}
	if err := decodeJWTClaims(rawToken, claims); err != nil {
		return nil, err
	}
	identity := &CasdoorIdentity{
		Sub:          claims.Sub,
		Username:     firstNonEmpty(claims.PreferredUsername, claims.Name, claims.Email, claims.Sub),
		DisplayName:  firstNonEmpty(claims.DisplayName, claims.Name, claims.PreferredUsername, claims.Email, "龙虾用户"),
		Email:        claims.Email,
		Avatar:       firstNonEmpty(claims.Picture, claims.Avatar),
		Organization: firstNonEmpty(claims.Owner, c.cfg.CasdoorOrganization),
	}
	if identity.Sub == "" {
		return nil, errors.New("casdoor token missing sub")
	}
	return identity, nil
}

func decodeJWTClaims(token string, dst any) error {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return errors.New("invalid jwt format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dst)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (a *App) handleAuthConfig(c *gin.Context) {
	if a.cfg.CasdoorEnabled() {
		c.JSON(http.StatusOK, gin.H{
			"mode":            "casdoor",
			"casdoor_enabled": true,
			"organization":    a.cfg.CasdoorOrganization,
			"application":     a.cfg.CasdoorApplication,
			"login_url":       "/api/auth/login/casdoor",
			"register_hint":   fmt.Sprintf("在统一认证页点击注册，注册后的用户会直接进入 %s 组织", a.cfg.CasdoorOrganization),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"mode":            "invite",
		"casdoor_enabled": false,
	})
}

func (a *App) handleCasdoorLogin(c *gin.Context) {
	if a.casdoor == nil || !a.cfg.CasdoorEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "casdoor disabled"})
		return
	}
	authURL, err := a.casdoor.LoginURL()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusFound, authURL)
}

func (a *App) handleCasdoorCallback(c *gin.Context) {
	if a.casdoor == nil || !a.cfg.CasdoorEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "casdoor disabled"})
		return
	}
	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		c.String(http.StatusBadRequest, "缺少 Casdoor 回调参数")
		return
	}
	if !a.casdoor.VerifyState(state) {
		c.String(http.StatusBadRequest, "Casdoor state 已失效，请返回登录页重试")
		return
	}

	tokenResp, err := a.casdoor.ExchangeCode(code)
	if err != nil {
		c.String(http.StatusBadGateway, "Casdoor 换取 token 失败: %v", err)
		return
	}
	identity, err := a.casdoor.ParseIdentity(tokenResp)
	if err != nil {
		c.String(http.StatusBadGateway, "Casdoor 用户解析失败: %v", err)
		return
	}

	user := a.findOrCreateCasdoorUser(identity)
	user.SessionToken = generateToken()
	user.LastLoginAt = time.Now().Format("2006-01-02 15:04:05")
	if err := a.store.SaveUser(user); err != nil {
		c.String(http.StatusInternalServerError, "保存本地用户失败: %v", err)
		return
	}

	a.renderLoginBridge(c, user)
}

func (a *App) findOrCreateCasdoorUser(identity *CasdoorIdentity) *sharing.User {
	user := a.store.GetUserByCasdoorSub(identity.Sub)
	settings := a.store.ReadSettings()
	if user == nil {
		user = &sharing.User{
			ID:          "user_" + shortID(),
			CreatedAt:   time.Now().Format("2006-01-02 15:04:05"),
			MaxLobsters: settings.DefaultMaxLobstersPerUser,
			AuthSource:  "casdoor",
			CasdoorSub:  identity.Sub,
		}
	}
	if user.MaxLobsters <= 0 {
		user.MaxLobsters = settings.DefaultMaxLobstersPerUser
	}
	user.AuthSource = "casdoor"
	user.Name = firstNonEmpty(identity.DisplayName, user.Name, identity.Username, identity.Email, "龙虾用户")
	user.CasdoorSub = identity.Sub
	user.CasdoorUsername = identity.Username
	user.CasdoorOrganization = firstNonEmpty(identity.Organization, a.cfg.CasdoorOrganization)
	user.Email = identity.Email
	user.Avatar = identity.Avatar
	return user
}

func (a *App) renderLoginBridge(c *gin.Context, user *sharing.User) {
	payloadBytes, _ := json.Marshal(map[string]any{
		"token": user.SessionToken,
		"user": map[string]any{
			"id":           user.ID,
			"name":         user.Name,
			"max_lobsters": user.MaxLobsters,
		},
	})
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>正在登录拼好虾...</title>
  <style>
    body{margin:0;font-family:-apple-system,BlinkMacSystemFont,"PingFang SC","Segoe UI",sans-serif;background:linear-gradient(135deg,#667eea 0%%,#764ba2 100%%);color:#fff;display:flex;align-items:center;justify-content:center;min-height:100vh}
    .card{background:rgba(255,255,255,.12);backdrop-filter:blur(14px);padding:32px 28px;border-radius:20px;box-shadow:0 20px 80px rgba(0,0,0,.18);text-align:center;max-width:420px}
    .title{font-size:24px;font-weight:700;margin:0 0 10px}
    .desc{opacity:.88;line-height:1.7;margin:0}
    .link{display:inline-block;margin-top:16px;color:#fff}
  </style>
</head>
<body>
  <div class="card">
    <div class="title">统一认证成功</div>
    <p class="desc">正在写入本地会话并跳转到拼好虾控制台，请稍候...</p>
    <a class="link" href="/#/pages/panel/index">如果没有自动跳转，请点这里</a>
  </div>
  <script>
    const payload = %s;
    localStorage.setItem("pc_user_token", payload.token || "");
    localStorage.setItem("pc_user_id", payload.user?.id || "");
    localStorage.setItem("pc_user_name", payload.user?.name || "");
    localStorage.setItem("pc_max_lobsters", String(payload.user?.max_lobsters || 3));
    window.location.replace("/#/pages/panel/index");
  </script>
</body>
</html>`, string(payloadBytes))
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
