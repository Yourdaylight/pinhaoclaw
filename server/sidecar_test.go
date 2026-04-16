package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pinhaoclaw/pinhaoclaw/config"
)

func newSidecarModeTestApp(t *testing.T, sidecarURL string) *App {
	t.Helper()
	dir := t.TempDir()
	app := NewApp(&config.Config{
		ShareClawHome:  dir,
		FrontendDir:    dir,
		AuthMode:       config.AuthModeSidecar,
		AuthSidecarURL: sidecarURL,
		PublicOrigin:   "http://localhost:9000",
	})
	return app
}

func TestHandleSidecarCallback_ForwardsQueryAndResponse(t *testing.T) {
	var gotQuery string
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/callback" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Sidecar-Test", "callback")
		_, _ = w.Write([]byte("<html>bridge</html>"))
	}))
	defer sidecar.Close()

	app := newSidecarModeTestApp(t, sidecar.URL)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/sidecar/callback?code=abc123&state=xyz789", nil)

	app.router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if gotQuery != "code=abc123&state=xyz789" {
		t.Fatalf("expected forwarded query, got %q", gotQuery)
	}
	if body := recorder.Body.String(); body != "<html>bridge</html>" {
		t.Fatalf("unexpected callback body: %q", body)
	}
	if header := recorder.Header().Get("X-Sidecar-Test"); header != "callback" {
		t.Fatalf("expected forwarded header, got %q", header)
	}
}

func TestHandleSidecarLogout_PostUsesHeaderToken(t *testing.T) {
	var verifyCalls int
	var logoutCalls int
	var gotLogoutToken string
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/verify":
			verifyCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"ok":true,"identity":{"sub":"sidecar-user","username":"tester","display_name":"Tester"}}`)
		case "/api/auth/logout":
			logoutCalls++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode logout payload: %v", err)
			}
			gotLogoutToken = payload["token"]
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"ok":true}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer sidecar.Close()

	app := newSidecarModeTestApp(t, sidecar.URL)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/sidecar/logout", nil)
	request.Header.Set("X-User-Token", "session-token-1")

	app.router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if verifyCalls != 1 {
		t.Fatalf("expected verify to be called once, got %d", verifyCalls)
	}
	if logoutCalls != 1 {
		t.Fatalf("expected logout to be called once, got %d", logoutCalls)
	}
	if gotLogoutToken != "session-token-1" {
		t.Fatalf("expected token to be forwarded in body, got %q", gotLogoutToken)
	}
	if !strings.Contains(recorder.Body.String(), `"ok":true`) {
		t.Fatalf("unexpected logout body: %q", recorder.Body.String())
	}
}

func TestHandleSidecarLogoutPage_RedirectsToRoot(t *testing.T) {
	app := newSidecarModeTestApp(t, "http://127.0.0.1:1")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/sidecar/logout", nil)

	app.router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", recorder.Code)
	}
	location, err := url.QueryUnescape(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatalf("failed to decode redirect: %v", err)
	}
	if location != "http://localhost:9000/" {
		t.Fatalf("unexpected redirect location: %q", location)
	}
}

func TestAuthenticateUserToken_SidecarVerifyCreatesLocalUser(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/verify" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":true,"identity":{"sub":"sidecar-user","username":"tester","display_name":"Tester","email":"tester@example.com"}}`)
	}))
	defer sidecar.Close()

	app := newSidecarModeTestApp(t, sidecar.URL)
	user, err := app.authenticateUserToken("sidecar-token")
	if err != nil {
		t.Fatalf("authenticateUserToken returned error: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.CasdoorSub != "sidecar-user" {
		t.Fatalf("expected mapped sidecar sub, got %q", user.CasdoorSub)
	}
	if user.Name != "Tester" {
		t.Fatalf("expected display name to be persisted, got %q", user.Name)
	}
}

func TestHandleSidecarLogoutComplete_ForwardsQueryAndResponse(t *testing.T) {
	var gotPath string
	var gotQuery string
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, "<html>logout-complete</html>")
	}))
	defer sidecar.Close()

	app := newSidecarModeTestApp(t, sidecar.URL)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/sidecar/logout-complete?redirect=https%3A%2F%2Fpinhaoclaw.example.com%2F", nil)

	app.router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if gotPath != "/api/auth/logout-complete" {
		t.Fatalf("expected logout complete path, got %q", gotPath)
	}
	if gotQuery != "redirect=https%3A%2F%2Fpinhaoclaw.example.com%2F" {
		t.Fatalf("expected forwarded query, got %q", gotQuery)
	}
	if body := recorder.Body.String(); body != "<html>logout-complete</html>" {
		t.Fatalf("unexpected logout complete body: %q", body)
	}
}

func TestRequireUser_SidecarVerifyHTTPFailureReturns401(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/verify" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		http.Error(w, "expired", http.StatusUnauthorized)
	}))
	defer sidecar.Close()

	app := newSidecarModeTestApp(t, sidecar.URL)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	request.Header.Set("X-User-Token", "invalid-token")

	app.router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "登录已过期，请重新登录") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestRequireUser_SidecarVerifyRejectedPayloadReturns401(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/verify" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":false,"error":"session expired"}`)
	}))
	defer sidecar.Close()

	app := newSidecarModeTestApp(t, sidecar.URL)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	request.Header.Set("X-User-Token", "invalid-token")

	app.router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "登录已过期，请重新登录") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}
