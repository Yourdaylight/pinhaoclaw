package sharing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store 基于 JSON 文件的持久化存储
type Store struct {
	dir string
	mu  sync.RWMutex
}

func NewStore(dataDir string) *Store {
	os.MkdirAll(dataDir, 0755)
	return &Store{dir: dataDir}
}

func (s *Store) path(filename string) string {
	return filepath.Join(s.dir, filename)
}

func (s *Store) readJSON(filename string, out any) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.path(filename))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (s *Store) writeJSON(filename string, val any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	users, _ := s.ReadUsers()
	users[u.ID] = u
	return s.writeJSON("users.json", users)
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
	NodeName            string `json:"node_name"`  // 节点名称（方便前端展示）
	Region              string `json:"region"`     // 华南 | 华北 | 华中 | 华东 | 境外
	Port                int    `json:"port"`
	Status              string `json:"status"`     // created | binding | running | stopped | error
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
	all, _ := s.ReadLobsters()
	all[l.ID] = l
	return s.writeJSON("lobsters.json", all)
}

func (s *Store) DeleteLobster(id string) error {
	all, _ := s.ReadLobsters()
	delete(all, id)
	return s.writeJSON("lobsters.json", all)
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
	ID           string `json:"id"`
	Name         string `json:"name"`
	Region       string `json:"region"`       // 华南 | 华北 | 华中 | 华东 | 境外
	Host         string `json:"host"`
	SSHPort      int    `json:"ssh_port"`
	SSHUser      string `json:"ssh_user"`
	SSHKeyPath   string `json:"ssh_key_path,omitempty"`
	SSHPassword  string `json:"ssh_password,omitempty"`
	Status       string `json:"status"` // online | offline | deploying
	MaxLobsters  int    `json:"max_lobsters"`
	CurrentCount int    `json:"current_count"`
	PicoClawPath string `json:"picoclaw_path"`
	RemoteHome   string `json:"remote_home"`
	CreatedAt    string `json:"created_at"`
}

func (s *Store) ReadNodes() (map[string]*Node, error) {
	result := make(map[string]*Node)
	if err := s.readJSON("nodes.json", &result); err != nil {
		return make(map[string]*Node), nil
	}
	return result, nil
}

func (s *Store) SaveNode(n *Node) error {
	all, _ := s.ReadNodes()
	all[n.ID] = n
	return s.writeJSON("nodes.json", all)
}

func (s *Store) DeleteNode(id string) error {
	all, _ := s.ReadNodes()
	delete(all, id)
	return s.writeJSON("nodes.json", all)
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
	Code      string `json:"code"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
	MaxUses   int    `json:"max_uses"`
	UsedCount int    `json:"used_count"`
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
	all, _ := s.ReadInvites()
	all[inv.Code] = inv
	return s.writeJSON("invites.json", all)
}

func (s *Store) DeleteInvite(code string) error {
	all, _ := s.ReadInvites()
	delete(all, code)
	return s.writeJSON("invites.json", all)
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
