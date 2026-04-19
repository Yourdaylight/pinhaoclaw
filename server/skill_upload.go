package server

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pinhaoclaw/pinhaoclaw/sharing"
)

var skillSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func (a *App) managedSkillsDir() string {
	return filepath.Join(a.store.Dir(), "skill_packages")
}

func (a *App) managedSkillDir(slug string) string {
	return filepath.Join(a.managedSkillsDir(), slug)
}

func normalizeSkillSlug(raw string) (string, error) {
	slug := strings.ToLower(strings.TrimSpace(raw))
	slug = strings.ReplaceAll(slug, " ", "-")
	if slug == "" {
		return "", fmt.Errorf("slug 必填")
	}
	if !skillSlugPattern.MatchString(slug) {
		return "", fmt.Errorf("slug 只能包含小写字母、数字、-、_")
	}
	return slug, nil
}

func splitCommaValues(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]bool)
	for _, item := range parts {
		value := strings.TrimSpace(item)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func cleanZipEntryName(name string) string {
	cleaned := strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if cleaned == "" {
		return ""
	}
	cleaned = path.Clean("/" + cleaned)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func detectSkillZipRoot(files []*zip.File) string {
	root := ""
	for _, file := range files {
		name := cleanZipEntryName(file.Name)
		if name == "" || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) < 2 {
			return ""
		}
		if root == "" {
			root = parts[0]
			continue
		}
		if parts[0] != root {
			return ""
		}
	}
	return root
}

func extractSkillZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.RemoveAll(destDir); err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	rootPrefix := detectSkillZipRoot(r.File)
	wroteAny := false
	for _, file := range r.File {
		name := cleanZipEntryName(file.Name)
		if name == "" || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		if rootPrefix != "" {
			if name == rootPrefix {
				continue
			}
			name = strings.TrimPrefix(name, rootPrefix+"/")
		}
		if name == "" {
			continue
		}
		targetPath := filepath.Join(destDir, filepath.FromSlash(name))
		rel, err := filepath.Rel(destDir, targetPath)
		if err != nil {
			return err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("zip contains invalid path: %s", file.Name)
		}
		info := file.FileInfo()
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("zip contains unsupported symlink: %s", file.Name)
		}
		if info.IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
		wroteAny = true
	}
	if !wroteAny {
		return fmt.Errorf("zip 包为空")
	}
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		return fmt.Errorf("zip 包缺少 SKILL.md")
	}
	return nil
}

func (a *App) removeManagedSkillAssets(entry *sharing.SkillRegistryEntry) error {
	if entry == nil || entry.Source.Type != "uploaded" || entry.Source.LocalDir == "" {
		return nil
	}
	managedRoot := a.managedSkillsDir()
	rel, err := filepath.Rel(managedRoot, entry.Source.LocalDir)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("refuse to remove unmanaged skill dir")
	}
	return os.RemoveAll(entry.Source.LocalDir)
}

func (a *App) handleAdminUploadSkill(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"ok": false, "message": "请上传 zip 包"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(fileHeader.Filename), ".zip") {
		c.JSON(400, gin.H{"ok": false, "message": "仅支持 zip 包"})
		return
	}

	slugInput := c.PostForm("slug")
	if strings.TrimSpace(slugInput) == "" {
		slugInput = strings.TrimSuffix(filepath.Base(fileHeader.Filename), filepath.Ext(fileHeader.Filename))
	}
	slug, err := normalizeSkillSlug(slugInput)
	if err != nil {
		c.JSON(400, gin.H{"ok": false, "message": err.Error()})
		return
	}

	tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("pinhaoclaw-skill-upload-%d.zip", time.Now().UnixNano()))
	if err := c.SaveUploadedFile(fileHeader, tmpZip); err != nil {
		c.JSON(500, gin.H{"ok": false, "message": "保存上传文件失败"})
		return
	}
	defer os.Remove(tmpZip)

	managedDir := a.managedSkillDir(slug)
	if err := os.MkdirAll(a.managedSkillsDir(), 0o755); err != nil {
		c.JSON(500, gin.H{"ok": false, "message": "初始化 skill 托管目录失败"})
		return
	}
	if err := extractSkillZip(tmpZip, managedDir); err != nil {
		_ = os.RemoveAll(managedDir)
		c.JSON(400, gin.H{"ok": false, "message": "解析 zip 失败: " + err.Error()})
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	existing := a.store.GetSkillRegistryEntry(slug)
	entry := &sharing.SkillRegistryEntry{
		Slug:        slug,
		DisplayName: strings.TrimSpace(c.PostForm("display_name")),
		Summary:     strings.TrimSpace(c.PostForm("summary")),
		Category:    strings.TrimSpace(c.PostForm("category")),
		Author:      strings.TrimSpace(c.PostForm("author")),
		Version:     strings.TrimSpace(c.PostForm("version")),
		Icon:        strings.TrimSpace(c.PostForm("icon")),
		Tags:        splitCommaValues(c.PostForm("tags")),
		Requires: &sharing.SkillRequires{
			Bins: splitCommaValues(c.PostForm("requires_bins")),
			Env:  splitCommaValues(c.PostForm("requires_env")),
		},
		Source: sharing.SkillSource{
			Type:     "uploaded",
			LocalDir: managedDir,
		},
		IsVerified: strings.EqualFold(strings.TrimSpace(c.PostForm("is_verified")), "true"),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if len(entry.Requires.Bins) == 0 && len(entry.Requires.Env) == 0 {
		entry.Requires = nil
	}
	if entry.DisplayName == "" {
		entry.DisplayName = slug
	}
	if existing != nil {
		entry.CreatedAt = existing.CreatedAt
		if entry.Summary == "" {
			entry.Summary = existing.Summary
		}
		if entry.Category == "" {
			entry.Category = existing.Category
		}
		if entry.Author == "" {
			entry.Author = existing.Author
		}
		if entry.Version == "" {
			entry.Version = existing.Version
		}
		if entry.Icon == "" {
			entry.Icon = existing.Icon
		}
		if len(entry.Tags) == 0 {
			entry.Tags = existing.Tags
		}
		if entry.Requires == nil {
			entry.Requires = existing.Requires
		}
		entry.IsVerified = entry.IsVerified || existing.IsVerified
	}
	if err := a.store.SaveSkillRegistryEntry(entry); err != nil {
		c.JSON(500, gin.H{"ok": false, "message": "保存 skill 失败"})
		return
	}
	c.JSON(201, gin.H{"ok": true, "skill": entry})
}
