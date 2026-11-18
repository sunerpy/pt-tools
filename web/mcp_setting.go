package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
)

// MCP 接入的开关与路径：外部的 AI 客户端经这个地址连 pt-tools 的 MCP 工具。设置存在 mcp_settings 表（ID 1），
// 读出来放进 s.mcpAccess；mcpGate 在路由前面按它分发，所以改了马上生效、不用重启。关着时这个地址（和默认的 /mcp）回 404
// 并写明原因，而不是落到网页的登录跳转。内置 AI 助手在进程内调工具，不经这个地址，不受开关影响。
// 开关与路径是授权边界：只能在网页上（session）改，配置网关不收。

// mcpPathMax 是路径最长多少个字符。
const mcpPathMax = 128

// mcpPathPattern 是路径的写法：/ 分级，每级是字母、数字与 . _ ~ -。
var mcpPathPattern = regexp.MustCompile(`^(/[A-Za-z0-9._~-]+)+$`)

// mcpReservedPrefixes 是 pt-tools 自己用的地址（接口、静态资源、登录），MCP 不能放在这些下面。
var mcpReservedPrefixes = []string{"/api", "/assets", "/static", "/login", "/logout"}

// mcpAccessState 是现在的开关与路径（mcpGate 读）。
type mcpAccessState struct {
	enabled bool
	path    string
}

// MCPSettingsView 是 GET /api/mcp/settings 的回应。
type MCPSettingsView struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
	// DefaultPath 是默认的路径（/mcp）
	DefaultPath string `json:"default_path"`
	// Tokens 是现在能连 MCP 的令牌个数（没过期、有 mcp:read 或 mcp:write）；读不到令牌时是 -1
	Tokens    int        `json:"tokens"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// MCPSettingsInput 是 PUT /api/mcp/settings 的请求体：path 为空时用默认的 /mcp。
type MCPSettingsInput struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

func (s *Server) registerMCPSettingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/mcp/settings", s.auth(s.apiMCPSettingsGet))
	mux.HandleFunc("PUT /api/mcp/settings", s.auth(s.apiMCPSettingsPut))
}

func mcpDB() *gorm.DB {
	if global.GlobalDB == nil {
		return nil
	}
	return global.GlobalDB.DB
}

// loadMCPSetting 读设置。还没有这一行时（新装或者刚升级上来）建一行：已经有带 MCP 权限的令牌就开着（已经接好的客户端不断），
// 否则关着。
func loadMCPSetting(ctx context.Context, db *gorm.DB) (models.MCPSetting, error) {
	var row models.MCPSetting
	if err := db.WithContext(ctx).Limit(1).Find(&row, 1).Error; err != nil {
		return row, fmt.Errorf("读取 MCP 接入设置失败: %w", err)
	}
	if row.ID != 0 {
		if row.Path == "" {
			row.Path = mcpPath
		}
		return row, nil
	}
	var n int64
	if db.Migrator().HasTable(&models.APIToken{}) {
		if err := db.WithContext(ctx).Model(&models.APIToken{}).Where("scopes LIKE ?", "%mcp:%").Count(&n).Error; err != nil {
			return row, fmt.Errorf("读取令牌失败: %w", err)
		}
	}
	row = models.MCPSetting{ID: 1, Enabled: n > 0, Path: mcpPath}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		return row, fmt.Errorf("保存 MCP 接入设置失败: %w", err)
	}
	if row.Enabled {
		global.GetSlogger().Infof("[MCP] 已有 %d 个带 MCP 权限的令牌：MCP 接入保持打开，地址 %s", n, row.Path)
	}
	return row, nil
}

// loadMCPAccess 在建路由时读设置放进 s.mcpAccess；读不到时关着（日志里写明原因）。
func (s *Server) loadMCPAccess() {
	st := &mcpAccessState{path: mcpPath}
	if db := mcpDB(); db != nil {
		row, err := loadMCPSetting(context.Background(), db)
		if err != nil {
			global.GetSlogger().Warnf("[MCP] %v：MCP 接入先关着", err)
		} else {
			st.enabled, st.path = row.Enabled, row.Path
		}
	}
	s.mcpAccess.Store(st)
}

// checkMCPPath 规整并检查路径：以 / 开头、末尾不带 /，不能放在 pt-tools 自己用的地址下面，也不能和已有的路由撞上。
func (s *Server) checkMCPPath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return mcpPath, nil
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "", errors.New("路径不能是 /：网页在那里")
	}
	if len(p) > mcpPathMax {
		return "", fmt.Errorf("路径最长 %d 个字符", mcpPathMax)
	}
	if !mcpPathPattern.MatchString(p) {
		return "", errors.New("路径只能有字母、数字与 . _ ~ -，用 / 分级")
	}
	for _, seg := range strings.Split(p[1:], "/") {
		if seg == "." || seg == ".." {
			return "", errors.New("路径里不能有 . 或 .. 这一级")
		}
	}
	lower := strings.ToLower(p)
	for _, prefix := range mcpReservedPrefixes {
		if lower == prefix || strings.HasPrefix(lower, prefix+"/") {
			return "", fmt.Errorf("%s 下面是 pt-tools 自己用的地址，换一个路径", prefix)
		}
	}
	if mux := s.configMux.Load(); mux != nil {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, p, nil)
		if err == nil {
			if _, pattern := mux.Handler(req); pattern != "" && pattern != "/" {
				return "", fmt.Errorf("和 pt-tools 已有的地址 %s 冲突，换一个路径", pattern)
			}
		}
	}
	return p, nil
}

// mcpTokenCount 是现在能连 MCP 的令牌个数；读不到时是 -1。
func (s *Server) mcpTokenCount(ctx context.Context) int {
	if s.tokens == nil {
		return -1
	}
	list, err := s.tokens.List(ctx)
	if err != nil {
		return -1
	}
	now := time.Now()
	n := 0
	for _, t := range list {
		if (t.Has(apitoken.ScopeMCPRead) || t.Has(apitoken.ScopeMCPWrite)) && (t.ExpiresAt == nil || t.ExpiresAt.After(now)) {
			n++
		}
	}
	return n
}

func (s *Server) mcpSettingsView(ctx context.Context, row models.MCPSetting) MCPSettingsView {
	v := MCPSettingsView{Enabled: row.Enabled, Path: row.Path, DefaultPath: mcpPath, Tokens: s.mcpTokenCount(ctx)}
	if !row.UpdatedAt.IsZero() {
		at := row.UpdatedAt
		v.UpdatedAt = &at
	}
	return v
}

// apiMCPSettingsGet handles GET /api/mcp/settings
func (s *Server) apiMCPSettingsGet(w http.ResponseWriter, r *http.Request) {
	db := mcpDB()
	if db == nil {
		http.Error(w, "数据库没有就绪", http.StatusServiceUnavailable)
		return
	}
	row, err := loadMCPSetting(r.Context(), db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.mcpSettingsView(r.Context(), row))
}

// apiMCPSettingsPut handles PUT /api/mcp/settings：保存并马上生效。
func (s *Server) apiMCPSettingsPut(w http.ResponseWriter, r *http.Request) {
	db := mcpDB()
	if db == nil {
		http.Error(w, "数据库没有就绪", http.StatusServiceUnavailable)
		return
	}
	var in MCPSettingsInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	path, err := s.checkMCPPath(in.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	row := models.MCPSetting{ID: 1, Enabled: in.Enabled, Path: path}
	if err := db.WithContext(r.Context()).Save(&row).Error; err != nil {
		http.Error(w, "保存 MCP 接入设置失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.mcpAccess.Store(&mcpAccessState{enabled: row.Enabled, path: row.Path})
	if row.Enabled {
		global.GetSlogger().Infof("[MCP] MCP 接入已打开，地址 %s", row.Path)
	} else {
		global.GetSlogger().Infof("[MCP] MCP 接入已关闭")
	}
	writeJSON(w, s.mcpSettingsView(r.Context(), row))
}

// mcpGate 在路由前面按开关与路径分发：开着时把路径上的请求交给 MCP；关着时这个路径与默认的 /mcp 回 404 并写明原因
// （MCP 客户端看得懂，不落到网页的登录跳转）。开着但换了路径时，默认的 /mcp 也回 404，不说新的路径在哪。
func (s *Server) mcpGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := s.mcpAccess.Load()
		if st == nil {
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case r.URL.Path == st.path && st.enabled && s.mcpHTTP != nil:
			s.mcpHTTP.ServeHTTP(w, r)
		case (r.URL.Path == st.path || r.URL.Path == mcpPath) && !st.enabled:
			http.Error(w, "MCP 接入没有打开：在 pt-tools 网页的「系统 → MCP 接入」里打开", http.StatusNotFound)
		case r.URL.Path == mcpPath && st.path != mcpPath:
			http.Error(w, "MCP 不在这个地址：到 pt-tools 网页的「系统 → MCP 接入」看现在的地址", http.StatusNotFound)
		default:
			next.ServeHTTP(w, r)
		}
	})
}
