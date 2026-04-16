package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAuthMode(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		hasSidecarURL bool
		want          AuthMode
		wantErr       bool
	}{
		{name: "auto invite", raw: "auto", hasSidecarURL: false, want: AuthModeInvite},
		{name: "auto sidecar", raw: "auto", hasSidecarURL: true, want: AuthModeSidecar},
		{name: "empty defaults to invite", raw: "", hasSidecarURL: false, want: AuthModeInvite},
		{name: "invite explicit", raw: "invite", hasSidecarURL: true, want: AuthModeInvite},
		{name: "sidecar explicit", raw: "sidecar", hasSidecarURL: false, want: AuthModeSidecar},
		{name: "invalid mode", raw: "casdoor", hasSidecarURL: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAuthMode(tt.raw, tt.hasSidecarURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveAuthMode returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestValidateRequiresSidecarURL(t *testing.T) {
	cfg := &Config{
		ShareClawHome: "/tmp/pinhaoclaw-home",
		FrontendDir:   "/tmp/pinhaoclaw-frontend",
		AuthMode:      AuthModeSidecar,
		PublicOrigin:  "http://localhost:9000",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error when sidecar mode has no URL")
	}
}

func TestPrepareForStartRequiresFrontendIndex(t *testing.T) {
	homeDir := t.TempDir()
	frontendDir := t.TempDir()

	cfg := &Config{
		ShareClawHome: homeDir,
		FrontendDir:   frontendDir,
		AuthMode:      AuthModeInvite,
		PublicOrigin:  "http://localhost:9000",
	}

	if err := cfg.PrepareForStart(); err == nil {
		t.Fatal("expected PrepareForStart to fail when index.html is missing")
	}
}

func TestPrepareForStartSucceedsWithWritableHomeAndFrontend(t *testing.T) {
	homeDir := t.TempDir()
	frontendDir := t.TempDir()
	indexFile := filepath.Join(frontendDir, "index.html")
	if err := os.WriteFile(indexFile, []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write index file: %v", err)
	}

	cfg := &Config{
		ShareClawHome: homeDir,
		FrontendDir:   frontendDir,
		AuthMode:      AuthModeInvite,
		PublicOrigin:  "http://localhost:9000",
	}

	if err := cfg.PrepareForStart(); err != nil {
		t.Fatalf("PrepareForStart returned error: %v", err)
	}
}
