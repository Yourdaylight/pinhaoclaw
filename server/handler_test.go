package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pinhaoclaw/pinhaoclaw/config"
	"github.com/pinhaoclaw/pinhaoclaw/sharing"
)

// ── 测试辅助 ──

func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	store := sharing.NewStore(dir)

	// 创建测试用户
	store.SaveUser(&sharing.User{
		ID:           "user_test01",
		Name:         "测试用户",
		SessionToken: "test-token-123",
		MaxLobsters:  3,
	})
	// 其他用户
	store.SaveUser(&sharing.User{
		ID:           "user_other01",
		Name:         "其他用户",
		SessionToken: "other-token-456",
		MaxLobsters:  3,
	})
	// 测试节点
	store.SaveNode(&sharing.Node{
		ID:          "node_test01",
		Name:        "测试节点",
		Host:        "1.2.3.4",
		RemoteHome:  "/opt/pinhaoclaw",
		Status:      "online",
		MaxLobsters: 10,
	})
	// 我的龙虾
	store.SaveLobster(&sharing.Lobster{
		ID:     "lobster_test01",
		UserID: "user_test01",
		Name:   "测试龙虾",
		NodeID: "node_test01",
		Port:   8101,
		Status: "stopped",
	})
	// 别人的龙虾
	store.SaveLobster(&sharing.Lobster{
		ID:     "lobster_other01",
		UserID: "user_other01",
		Name:   "别人的龙虾",
		NodeID: "node_test01",
		Port:   8102,
		Status: "running",
	})

	cfg := &config.Config{
		ShareClawHome: dir,
		FrontendDir:   dir, // 不实际使用前端
		AuthMode:      config.AuthModeInvite,
	}
	app := NewApp(cfg)
	return app, dir
}

// ── P0-3: WebSocket 权限校验 ──

func TestBindWeixinWS_NotOwner_Returns403(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/ws/bind/lobster_other01?token=test-token-123", nil)
	c.Params = gin.Params{{Key: "id", Value: "lobster_other01"}}

	app.handleBindWeixinWS(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-owner, got %d", w.Code)
	}
}

func TestBindWeixinWS_NoToken_Returns401(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/ws/bind/lobster_test01", nil)
	c.Params = gin.Params{{Key: "id", Value: "lobster_test01"}}

	app.handleBindWeixinWS(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no token, got %d", w.Code)
	}
}

func TestBindWeixinWS_InvalidToken_Returns401(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/ws/bind/lobster_test01?token=invalid", nil)
	c.Params = gin.Params{{Key: "id", Value: "lobster_test01"}}

	app.handleBindWeixinWS(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", w.Code)
	}
}

func TestBindWeixinWS_NonexistentLobster_Returns404(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/ws/bind/nonexistent?token=test-token-123", nil)
	c.Params = gin.Params{{Key: "id", Value: "nonexistent"}}

	app.handleBindWeixinWS(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent lobster, got %d", w.Code)
	}
}

// ── P0-2: Start 龙虾权限检查 ──

func TestStartLobster_NotOwner_Returns404(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/lobsters/lobster_other01/start", nil)
	c.Params = gin.Params{{Key: "id", Value: "lobster_other01"}}

	// 模拟 requireUser 中间件设置的用户
	c.Set("user", &sharing.User{ID: "user_test01"})

	app.handleStartLobster(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-owner, got %d", w.Code)
	}
}

func TestStartLobster_LobsterNotFound_Returns404(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/lobsters/nonexistent/start", nil)
	c.Params = gin.Params{{Key: "id", Value: "nonexistent"}}

	c.Set("user", &sharing.User{ID: "user_test01"})

	app.handleStartLobster(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent lobster, got %d", w.Code)
	}
}

func TestGetLobster_NotOwner_Returns404(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/lobsters/lobster_other01", nil)
	c.Params = gin.Params{{Key: "id", Value: "lobster_other01"}}

	c.Set("user", &sharing.User{ID: "user_test01"})

	app.handleGetLobster(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-owner, got %d", w.Code)
	}
}

// ── P1-2: SSE JSON 安全 ──

func TestWriteSSE_JsonEscape(t *testing.T) {
	// 验证 json.Marshal 正确处理特殊字符
	data := map[string]string{
		"stage":   `login "step"`,
		"message": `user said: {"key": "value"}`,
	}
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Errorf("SSE data is not valid JSON: %v, raw: %s", err, string(jsonBytes))
	}
	if parsed["stage"] != `login "step"` {
		t.Errorf("stage not preserved, got: %s", parsed["stage"])
	}
	if parsed["message"] != `user said: {"key": "value"}` {
		t.Errorf("message not preserved, got: %s", parsed["message"])
	}
}

// ── 用户认证中间件 ──

func TestRequireUser_ValidToken(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/lobsters", nil)
	c.Request.Header.Set("X-User-Token", "test-token-123")

	app.requireUser()(c)
	if c.IsAborted() {
		t.Error("valid token should not be aborted")
	}
}

func TestRequireUser_InvalidToken_Aborts(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/lobsters", nil)
	c.Request.Header.Set("X-User-Token", "invalid-token")

	app.requireUser()(c)
	if !c.IsAborted() {
		t.Error("invalid token should be aborted")
	}
}

func TestRequireUser_NoToken_Aborts(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/lobsters", nil)

	app.requireUser()(c)
	if !c.IsAborted() {
		t.Error("no token should be aborted")
	}
}

// ── 龙虾所有权校验（所有操作） ──

func TestStopLobster_NotOwner_Returns404(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/lobsters/lobster_other01/stop", nil)
	c.Params = gin.Params{{Key: "id", Value: "lobster_other01"}}

	c.Set("user", &sharing.User{ID: "user_test01"})

	app.handleStopLobster(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-owner stop, got %d", w.Code)
	}
}

func TestDeleteLobster_NotOwner_Returns404(t *testing.T) {
	app, _ := newTestApp(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/lobsters/lobster_other01", nil)
	c.Params = gin.Params{{Key: "id", Value: "lobster_other01"}}

	c.Set("user", &sharing.User{ID: "user_test01"})

	app.handleDeleteLobster(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-owner delete, got %d", w.Code)
	}
}

func TestCreateLobster_ReleasesLockAfterInvalidRequest(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(&config.Config{
		ShareClawHome: dir,
		FrontendDir:   dir,
		AuthMode:      config.AuthModeInvite,
	})

	invalidRecorder := httptest.NewRecorder()
	invalidCtx, _ := gin.CreateTestContext(invalidRecorder)
	invalidCtx.Request = httptest.NewRequest(http.MethodPost, "/api/lobsters", bytes.NewBufferString("{"))
	invalidCtx.Request.Header.Set("Content-Type", "application/json")
	invalidCtx.Set("user", &sharing.User{ID: "user_test01", MaxLobsters: 3})

	app.handleCreateLobster(invalidCtx)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid body, got %d", invalidRecorder.Code)
	}

	done := make(chan int, 1)
	go func() {
		validRecorder := httptest.NewRecorder()
		validCtx, _ := gin.CreateTestContext(validRecorder)
		validCtx.Request = httptest.NewRequest(http.MethodPost, "/api/lobsters", bytes.NewBufferString(`{"name":"test"}`))
		validCtx.Request.Header.Set("Content-Type", "application/json")
		validCtx.Set("user", &sharing.User{ID: "user_test01", MaxLobsters: 3})
		app.handleCreateLobster(validCtx)
		done <- validRecorder.Code
	}()

	select {
	case code := <-done:
		if code != http.StatusServiceUnavailable {
			t.Fatalf("expected second request to finish with 503, got %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second create request blocked, mutex was not released")
	}
}

func TestHandleAdminUploadSkill_SavesManagedSkillAndRegistry(t *testing.T) {
	app, dir := newTestApp(t)

	zipPath := filepath.Join(t.TempDir(), "demo-skill.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(zipFile)
	skillFile, err := zw.Create("demo-skill/SKILL.md")
	if err != nil {
		t.Fatalf("create skill entry: %v", err)
	}
	if _, err := skillFile.Write([]byte("# Demo Skill")); err != nil {
		t.Fatalf("write skill entry: %v", err)
	}
	dataFile, err := zw.Create("demo-skill/scripts/run.sh")
	if err != nil {
		t.Fatalf("create data entry: %v", err)
	}
	if _, err := dataFile.Write([]byte("echo demo")); err != nil {
		t.Fatalf("write data entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("slug", "demo-skill")
	_ = writer.WriteField("display_name", "Demo Skill")
	_ = writer.WriteField("summary", "uploaded from zip")
	_ = writer.WriteField("version", "1.0.0")
	part, err := writer.CreateFormFile("file", filepath.Base(zipPath))
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	zipBytes, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("read zip bytes: %v", err)
	}
	if _, err := part.Write(zipBytes); err != nil {
		t.Fatalf("write multipart zip: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/skills/upload", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	app.handleAdminUploadSkill(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	entry := app.store.GetSkillRegistryEntry("demo-skill")
	if entry == nil {
		t.Fatal("expected skill registry entry to be created")
	}
	if entry.Source.Type != "uploaded" {
		t.Fatalf("expected uploaded source type, got %q", entry.Source.Type)
	}
	if entry.Source.LocalDir != filepath.Join(dir, "skill_packages", "demo-skill") {
		t.Fatalf("unexpected managed skill dir: %s", entry.Source.LocalDir)
	}
	if _, err := os.Stat(filepath.Join(entry.Source.LocalDir, "SKILL.md")); err != nil {
		t.Fatalf("expected managed SKILL.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(entry.Source.LocalDir, "scripts", "run.sh")); err != nil {
		t.Fatalf("expected managed script file: %v", err)
	}
}
