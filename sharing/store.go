package sharing

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Store 基于 JSON 文件的持久化存储（flock 文件锁保证并发安全）
type Store struct {
	dir        string
	encryptKey []byte // AES-GCM key for encrypting sensitive fields
}

func NewStore(dataDir string) *Store {
	os.MkdirAll(dataDir, 0755)
	s := &Store{dir: dataDir}
	// Load encryption key from environment variable
	if keyHex := os.Getenv("PINHAOCLAW_ENCRYPT_KEY"); keyHex != "" {
		if key, err := hex.DecodeString(keyHex); err == nil && len(key) == 32 {
			s.encryptKey = key
		}
	}
	return s
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) path(filename string) string {
	return filepath.Join(s.dir, filename)
}

// lockFile 获取文件锁（排他锁），返回解锁函数
func (s *Store) lockFile(filename string) (func(), error) {
	lockPath := s.path(filename + ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", filename, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("flock %s: %w", filename, err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// WithLock 在文件锁保护下执行原子操作（用于 read-modify-write 场景）
func (s *Store) WithLock(filename string, fn func() error) error {
	unlock, err := s.lockFile(filename)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

// readFile 读取 JSON 文件（不加锁，调用者需通过 WithLock 加锁）
func (s *Store) readFile(filename string, out any) error {
	data, err := os.ReadFile(s.path(filename))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// writeFile 写入 JSON 文件（不加锁，调用者需通过 WithLock 加锁）
func (s *Store) writeFile(filename string, val any) error {
	p := s.path(filename)
	tmp := p + ".tmp"
	data, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filename, err)
	}
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}
	return os.Rename(tmp, p)
}

// readJSON 加锁读取 JSON 文件（独立读操作）
func (s *Store) readJSON(filename string, out any) error {
	unlock, err := s.lockFile(filename)
	if err != nil {
		return err
	}
	defer unlock()
	return s.readFile(filename, out)
}

// writeJSON 加锁写入 JSON 文件（独立写操作）
func (s *Store) writeJSON(filename string, val any) error {
	unlock, err := s.lockFile(filename)
	if err != nil {
		return err
	}
	defer unlock()
	return s.writeFile(filename, val)
}

// ── Encryption helpers ──────────────────────────────────

const encryptedPrefix = "enc:"

// encryptField encrypts a plaintext string using AES-GCM.
// Returns a string prefixed with "enc:" to indicate it's encrypted.
// If no encryption key is configured, returns the plaintext unchanged.
func (s *Store) encryptField(plaintext string) string {
	if s.encryptKey == nil || plaintext == "" {
		return plaintext
	}
	block, err := aes.NewCipher(s.encryptKey)
	if err != nil {
		return plaintext
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return plaintext
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return plaintext
	}
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return encryptedPrefix + hex.EncodeToString(ciphertext)
}

// decryptField decrypts a string that was encrypted with encryptField.
// If the string doesn't have the "enc:" prefix, it's returned as-is (backward compatible).
func (s *Store) decryptField(encrypted string) string {
	if !strings.HasPrefix(encrypted, encryptedPrefix) {
		return encrypted
	}
	if s.encryptKey == nil {
		return "" // Can't decrypt without key
	}
	data, err := hex.DecodeString(strings.TrimPrefix(encrypted, encryptedPrefix))
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(s.encryptKey)
	if err != nil {
		return ""
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return ""
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return ""
	}
	return string(plaintext)
}

// ── User ──────────────────────────────────────────────

type User struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	InviteCode          string `json:"invite_code,omitempty"`
	CreatedAt           string `json:"created_at"`
	LastLoginAt         string `json:"last_login_at"`
	MaxLobsters         int    `json:"max_lobsters"`
	SessionToken        string `json:"session_token,omitempty"`
	AuthSource          string `json:"auth_source,omitempty"`
	CasdoorSub          string `json:"casdoor_sub,omitempty"`
	CasdoorUsername     string `json:"casdoor_username,omitempty"`
	CasdoorOrganization string `json:"casdoor_organization,omitempty"`
	Email               string `json:"email,omitempty"`
	Avatar              string `json:"avatar,omitempty"`
}

func (s *Store) ReadUsers() (map[string]*User, error) {
	result := make(map[string]*User)
	if err := s.readJSON("users.json", &result); err != nil {
		return make(map[string]*User), nil
	}
	return result, nil
}

func (s *Store) SaveUser(u *User) error {
	return s.WithLock("users.json", func() error {
		users := make(map[string]*User)
		_ = s.readFile("users.json", &users)
		users[u.ID] = u
		return s.writeFile("users.json", users)
	})
}

func (s *Store) GetUserByInviteCode(code string) *User {
	users, _ := s.ReadUsers()
	for _, u := range users {
		if u.InviteCode == code {
			return u
		}
	}
	return nil
}

func (s *Store) GetUser(id string) *User {
	users, _ := s.ReadUsers()
	return users[id]
}

func (s *Store) GetUserByCasdoorSub(sub string) *User {
	users, _ := s.ReadUsers()
	for _, u := range users {
		if u.CasdoorSub == sub {
			return u
		}
	}
	return nil
}

// ── Lobster（龙虾 = 隔离的 picoclaw 实例） ──────────

type Lobster struct {
	ID                  string `json:"id"`
	UserID              string `json:"user_id"`
	Name                string `json:"name"`
	NodeID              string `json:"node_id"`
	NodeName            string `json:"node_name"` // 节点名称（方便前端展示）
	Region              string `json:"region"`    // 华南 | 华北 | 华中 | 华东 | 境外
	Port                int    `json:"port"`
	Status              string `json:"status"` // created | binding | running | stopped | error
	WeixinBound         bool   `json:"weixin_bound"`
	WeixinName          string `json:"weixin_name,omitempty"`
	BoundAt             string `json:"bound_at,omitempty"`
	CreatedAt           string `json:"created_at"`
	MonthlyTokenLimit   int64  `json:"monthly_token_limit"`
	MonthlyTokenUsed    int64  `json:"monthly_token_used"`
	MonthlySpaceLimitMB int64  `json:"monthly_space_limit_mb"`
	MonthlySpaceUsedMB  int64  `json:"monthly_space_used_mb"`
	QuotaResetMonth     string `json:"quota_reset_month"`
}

func (l *Lobster) EnsureMonthlyReset() bool {
	m := time.Now().Format("2006-01")
	if l.QuotaResetMonth != m {
		l.MonthlyTokenUsed = 0
		l.MonthlySpaceUsedMB = 0
		l.QuotaResetMonth = m
		return true
	}
	return false
}

func (l *Lobster) TokenRemaining() int64 {
	if l.MonthlyTokenLimit <= 0 {
		return 999999999
	}
	r := l.MonthlyTokenLimit - l.MonthlyTokenUsed
	if r < 0 {
		return 0
	}
	return r
}

func (l *Lobster) SpaceRemaining() int64 {
	if l.MonthlySpaceLimitMB <= 0 {
		return 999999
	}
	r := l.MonthlySpaceLimitMB - l.MonthlySpaceUsedMB
	if r < 0 {
		return 0
	}
	return r
}

func (s *Store) ReadLobsters() (map[string]*Lobster, error) {
	result := make(map[string]*Lobster)
	if err := s.readJSON("lobsters.json", &result); err != nil {
		return make(map[string]*Lobster), nil
	}
	return result, nil
}

func (s *Store) SaveLobster(l *Lobster) error {
	return s.WithLock("lobsters.json", func() error {
		all := make(map[string]*Lobster)
		_ = s.readFile("lobsters.json", &all)
		all[l.ID] = l
		return s.writeFile("lobsters.json", all)
	})
}

func (s *Store) DeleteLobster(id string) error {
	return s.WithLock("lobsters.json", func() error {
		all := make(map[string]*Lobster)
		_ = s.readFile("lobsters.json", &all)
		delete(all, id)
		return s.writeFile("lobsters.json", all)
	})
}

func (s *Store) GetLobster(id string) *Lobster {
	all, _ := s.ReadLobsters()
	return all[id]
}

func (s *Store) GetLobstersByUser(userID string) []*Lobster {
	all, _ := s.ReadLobsters()
	var result []*Lobster
	for _, l := range all {
		if l.UserID == userID {
			result = append(result, l)
		}
	}
	return result
}

func (s *Store) CountLobstersByUser(userID string) int {
	return len(s.GetLobstersByUser(userID))
}

// ── Node（云节点） ────────────────────────────────────

type Node struct {
	ID                 string `json:"id"`
	Type               string `json:"type,omitempty"` // ssh | local
	Name               string `json:"name"`
	Region             string `json:"region"` // 华南 | 华北 | 华中 | 华东 | 境外
	Host               string `json:"host"`
	SSHPort            int    `json:"ssh_port"`
	SSHUser            string `json:"ssh_user"`
	SSHAuthType        string `json:"ssh_auth_type,omitempty"` // password | key_path | private_key
	SSHKeyPath         string `json:"ssh_key_path,omitempty"`
	SSHPrivateKey      string `json:"ssh_private_key,omitempty"`
	SSHCertificatePath string `json:"ssh_certificate_path,omitempty"`
	SSHKeyPassphrase   string `json:"ssh_key_passphrase,omitempty"`
	SSHPassword        string `json:"ssh_password,omitempty"`
	Status             string `json:"status"` // online | offline | deploying
	MaxLobsters        int    `json:"max_lobsters"`
	CurrentCount       int    `json:"current_count"`
	PicoClawPath       string `json:"picoclaw_path"`
	RemoteHome         string `json:"remote_home"`
	CreatedAt          string `json:"created_at"`
}

func (s *Store) ReadNodes() (map[string]*Node, error) {
	result := make(map[string]*Node)
	if err := s.readJSON("nodes.json", &result); err != nil {
		return make(map[string]*Node), nil
	}
	// Decrypt sensitive SSH credentials on read.
	for _, n := range result {
		n.SSHPassword = s.decryptField(n.SSHPassword)
		n.SSHPrivateKey = s.decryptField(n.SSHPrivateKey)
		n.SSHKeyPassphrase = s.decryptField(n.SSHKeyPassphrase)
	}
	return result, nil
}

func (s *Store) SaveNode(n *Node) error {
	return s.WithLock("nodes.json", func() error {
		all := make(map[string]*Node)
		_ = s.readFile("nodes.json", &all)

		toSave := *n
		toSave.SSHPassword = s.encryptField(toSave.SSHPassword)
		toSave.SSHPrivateKey = s.encryptField(toSave.SSHPrivateKey)
		toSave.SSHKeyPassphrase = s.encryptField(toSave.SSHKeyPassphrase)

		all[n.ID] = &toSave
		return s.writeFile("nodes.json", all)
	})
}

func (s *Store) DeleteNode(id string) error {
	return s.WithLock("nodes.json", func() error {
		all := make(map[string]*Node)
		_ = s.readFile("nodes.json", &all)
		delete(all, id)
		return s.writeFile("nodes.json", all)
	})
}

func (s *Store) GetNode(id string) *Node {
	all, _ := s.ReadNodes()
	return all[id]
}

// SelectNode 选择最空闲的在线节点（可按 region 过滤，空字符串=不限）
func (s *Store) SelectNode(region string) *Node {
	all, _ := s.ReadNodes()
	var best *Node
	for _, n := range all {
		if n.Status != "online" {
			continue
		}
		if n.MaxLobsters > 0 && n.CurrentCount >= n.MaxLobsters {
			continue
		}
		if region != "" && n.Region != region {
			continue
		}
		if best == nil || n.CurrentCount < best.CurrentCount {
			best = n
		}
	}
	return best
}

// ListOnlineRegions 返回当前有在线节点的区域列表
func (s *Store) ListOnlineRegions() []string {
	all, _ := s.ReadNodes()
	seen := make(map[string]bool)
	var regions []string
	for _, n := range all {
		if n.Status == "online" && n.Region != "" && !seen[n.Region] {
			seen[n.Region] = true
			regions = append(regions, n.Region)
		}
	}
	return regions
}

// ── Invite（邀请码） ──────────────────────────────────

type Invite struct {
	Code      string   `json:"code"`
	CreatedBy string   `json:"created_by"`
	CreatedAt string   `json:"created_at"`
	MaxUses   int      `json:"max_uses"`
	UsedCount int      `json:"used_count"`
	UsedBy    []string `json:"used_by,omitempty"`
}

func (s *Store) ReadInvites() (map[string]*Invite, error) {
	result := make(map[string]*Invite)
	if err := s.readJSON("invites.json", &result); err != nil {
		return make(map[string]*Invite), nil
	}
	return result, nil
}

func (s *Store) SaveInvite(inv *Invite) error {
	return s.WithLock("invites.json", func() error {
		all := make(map[string]*Invite)
		_ = s.readFile("invites.json", &all)
		all[inv.Code] = inv
		return s.writeFile("invites.json", all)
	})
}

func (s *Store) DeleteInvite(code string) error {
	return s.WithLock("invites.json", func() error {
		all := make(map[string]*Invite)
		_ = s.readFile("invites.json", &all)
		delete(all, code)
		return s.writeFile("invites.json", all)
	})
}

func (s *Store) GetInvite(code string) *Invite {
	all, _ := s.ReadInvites()
	return all[code]
}

// ── Settings（全局设置） ──────────────────────────────

type Settings struct {
	DefaultMonthlyTokenLimit   int64  `json:"default_monthly_token_limit"`
	DefaultMonthlySpaceLimitMB int64  `json:"default_monthly_space_limit_mb"`
	DefaultMaxLobstersPerUser  int    `json:"default_max_lobsters_per_user"`
	PicoclawPackagePath        string `json:"picoclaw_package_path,omitempty"`
	AdminPassword              string `json:"-"`
}

var DefaultSettings = Settings{
	DefaultMonthlyTokenLimit:   1000000,
	DefaultMonthlySpaceLimitMB: 2048,
	DefaultMaxLobstersPerUser:  3,
}

func (s *Store) ReadSettings() *Settings {
	result := &Settings{}
	if err := s.readJSON("settings.json", result); err != nil {
		dup := DefaultSettings
		return &dup
	}
	if result.DefaultMonthlyTokenLimit <= 0 {
		result.DefaultMonthlyTokenLimit = DefaultSettings.DefaultMonthlyTokenLimit
	}
	if result.DefaultMonthlySpaceLimitMB <= 0 {
		result.DefaultMonthlySpaceLimitMB = DefaultSettings.DefaultMonthlySpaceLimitMB
	}
	if result.DefaultMaxLobstersPerUser <= 0 {
		result.DefaultMaxLobstersPerUser = DefaultSettings.DefaultMaxLobstersPerUser
	}
	return result
}

func (s *Store) WriteSettings(settings *Settings) error {
	return s.writeJSON("settings.json", settings)
}

// ── SkillRegistry (Skill Library Entry) ──────────

type SkillRequires struct {
	Bins []string `json:"bins,omitempty"`
	Env  []string `json:"env,omitempty"`
}

type SkillSource struct {
	Type     string `json:"type"`
	Repo     string `json:"repo,omitempty"`
	ClawHub  string `json:"clawhub,omitempty"`
	LocalDir string `json:"local_dir,omitempty"`
}

type SkillRegistryEntry struct {
	Slug        string         `json:"slug"`
	DisplayName string         `json:"display_name"`
	Summary     string         `json:"summary"`
	Category    string         `json:"category"`
	Author      string         `json:"author"`
	Version     string         `json:"version"`
	Icon        string         `json:"icon,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Requires    *SkillRequires `json:"requires,omitempty"`
	Source      SkillSource    `json:"source"`
	IsVerified  bool           `json:"is_verified"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

func (s *Store) ReadSkillRegistry() (map[string]*SkillRegistryEntry, error) {
	result := make(map[string]*SkillRegistryEntry)
	if err := s.readJSON("skill_registry.json", &result); err != nil {
		return make(map[string]*SkillRegistryEntry), nil
	}
	return result, nil
}

func (s *Store) SaveSkillRegistryEntry(entry *SkillRegistryEntry) error {
	return s.WithLock("skill_registry.json", func() error {
		all := make(map[string]*SkillRegistryEntry)
		_ = s.readFile("skill_registry.json", &all)
		all[entry.Slug] = entry
		return s.writeFile("skill_registry.json", all)
	})
}

func (s *Store) DeleteSkillRegistryEntry(slug string) error {
	return s.WithLock("skill_registry.json", func() error {
		all := make(map[string]*SkillRegistryEntry)
		_ = s.readFile("skill_registry.json", &all)
		delete(all, slug)
		return s.writeFile("skill_registry.json", all)
	})
}

func (s *Store) GetSkillRegistryEntry(slug string) *SkillRegistryEntry {
	all, _ := s.ReadSkillRegistry()
	return all[slug]
}

// ── LobsterSkill (Skill installed on a lobster) ──────────

type LobsterSkill struct {
	Slug        string `json:"slug"`
	Version     string `json:"version"`
	InstalledAt string `json:"installed_at"`
}

func lobsterSkillsFilename(lobsterID string) string {
	return "lobster_skills_" + lobsterID + ".json"
}

func (s *Store) ReadLobsterSkills(lobsterID string) ([]*LobsterSkill, error) {
	result := make([]*LobsterSkill, 0)
	if err := s.readJSON(lobsterSkillsFilename(lobsterID), &result); err != nil {
		return make([]*LobsterSkill, 0), nil
	}
	return result, nil
}

func (s *Store) SaveLobsterSkill(lobsterID string, ls *LobsterSkill) error {
	fname := lobsterSkillsFilename(lobsterID)
	return s.WithLock(fname, func() error {
		all := make([]*LobsterSkill, 0)
		_ = s.readFile(fname, &all)
		found := false
		for i, existing := range all {
			if existing.Slug == ls.Slug {
				all[i] = ls
				found = true
				break
			}
		}
		if !found {
			all = append(all, ls)
		}
		return s.writeFile(fname, all)
	})
}

func (s *Store) RemoveLobsterSkill(lobsterID string, slug string) error {
	fname := lobsterSkillsFilename(lobsterID)
	return s.WithLock(fname, func() error {
		all := make([]*LobsterSkill, 0)
		_ = s.readFile(fname, &all)
		filtered := make([]*LobsterSkill, 0, len(all))
		for _, ls := range all {
			if ls.Slug != slug {
				filtered = append(filtered, ls)
			}
		}
		return s.writeFile(fname, filtered)
	})
}
