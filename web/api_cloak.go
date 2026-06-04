package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/cloakdriver"
)

const (
	cloakDefaultTestTimeoutMs = 5000

	cloakCatSuccess       = "success"
	cloakCatDNSFail       = "dns_fail"
	cloakCatConnRefused   = "conn_refused"
	cloakCatTimeout       = "timeout"
	cloakCatAuthFail      = "auth_fail"
	cloakCatNotFound      = "not_found"
	cloakCatServerError   = "server_error"
	cloakCatProtocolError = "protocol_error"
	cloakCatUnknown       = "unknown"
)

type cloakConfigGetResponse struct {
	Endpoint       string  `json:"endpoint"`
	HasToken       bool    `json:"has_token"`
	ManagerVersion *string `json:"manager_version,omitempty"`
}

type cloakConfigPutRequest struct {
	Endpoint string  `json:"endpoint"`
	Token    *string `json:"token,omitempty"`
}

type cloakTestRequest struct {
	Endpoint  string `json:"endpoint,omitempty"`
	Token     string `json:"token,omitempty"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

type cloakTestResponse struct {
	Category       string `json:"category"`
	Message        string `json:"message"`
	ManagerVersion string `json:"manager_version,omitempty"`
}

func (s *Server) apiCloakConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleCloakConfigGet(w, r)
	case http.MethodPut:
		s.handleCloakConfigPut(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCloakConfigGet(w http.ResponseWriter, _ *http.Request) {
	if s.store == nil {
		writeJSONError(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	snap, err := s.store.GetCloakConfig()
	if err != nil {
		writeJSONError(w, fmt.Sprintf("加载 CloakBrowser 配置失败: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cloakConfigGetResponse{
		Endpoint: snap.Endpoint,
		HasToken: snap.HasToken,
	})
}

func (s *Server) handleCloakConfigPut(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSONError(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	defer func() {
		if r.Body != nil {
			_ = r.Body.Close()
		}
	}()
	var req cloakConfigPutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, fmt.Sprintf("无效请求: %v", err), http.StatusBadRequest)
		return
	}
	endpoint := strings.TrimSpace(req.Endpoint)
	if endpoint == "" {
		writeJSONError(w, "endpoint 不能为空", http.StatusBadRequest)
		return
	}
	token := ""
	if req.Token != nil {
		token = *req.Token
	}
	if err := s.store.SaveCloakConfig(endpoint, token, false); err != nil {
		writeJSONError(w, fmt.Sprintf("保存 CloakBrowser 配置失败: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) apiCloakTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.store == nil {
		writeJSONError(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	defer func() {
		if r.Body != nil {
			_ = r.Body.Close()
		}
	}()
	var req cloakTestRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, fmt.Sprintf("无效请求: %v", err), http.StatusBadRequest)
			return
		}
	}
	endpoint := strings.TrimSpace(req.Endpoint)
	token := req.Token
	if endpoint == "" {
		ep, err := s.store.GetCloakEndpoint()
		if err != nil {
			writeJSONError(w, fmt.Sprintf("加载 CloakBrowser 配置失败: %v", err), http.StatusInternalServerError)
			return
		}
		endpoint = ep
	}
	if token == "" {
		tok, err := s.store.GetCloakToken()
		if err != nil {
			writeJSONError(w, fmt.Sprintf("加载 CloakBrowser token 失败: %v", err), http.StatusInternalServerError)
			return
		}
		token = tok
	}
	if endpoint == "" {
		writeJSON(w, cloakTestResponse{
			Category: cloakCatUnknown,
			Message:  "未配置 endpoint，请先填写 CloakBrowser-Manager 地址",
		})
		return
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if req.TimeoutMs <= 0 {
		timeout = cloakDefaultTestTimeoutMs * time.Millisecond
	}
	client := cloakdriver.NewManagerClient(endpoint, token, timeout)
	ctx, cancel := context.WithTimeout(r.Context(), timeout+500*time.Millisecond)
	defer cancel()

	info, err := client.ManagerStatusFull(ctx)
	resp := classifyCloakTestResult(info, err)
	writeJSON(w, resp)
}

func classifyCloakTestResult(info *cloakdriver.ManagerStatusInfo, err error) cloakTestResponse {
	if err == nil {
		version := ""
		if info != nil {
			version = info.Version
		}
		return cloakTestResponse{
			Category:       cloakCatSuccess,
			Message:        "连接成功",
			ManagerVersion: version,
		}
	}
	switch {
	case errors.Is(err, cloakdriver.ErrManagerAuthFailed):
		return cloakTestResponse{Category: cloakCatAuthFail, Message: "认证失败：请检查 token 是否正确"}
	case errors.Is(err, cloakdriver.ErrManagerNotFound):
		return cloakTestResponse{Category: cloakCatNotFound, Message: "未找到 /api/status：请确认 CloakBrowser-Manager 版本"}
	case errors.Is(err, cloakdriver.ErrManagerServerError):
		return cloakTestResponse{Category: cloakCatServerError, Message: "服务端错误（5xx）：CloakBrowser-Manager 内部异常"}
	case errors.Is(err, cloakdriver.ErrManagerDNSFailed):
		return cloakTestResponse{Category: cloakCatDNSFail, Message: "DNS 解析失败：请检查 endpoint 域名"}
	case errors.Is(err, cloakdriver.ErrManagerConnRefused):
		return cloakTestResponse{Category: cloakCatConnRefused, Message: "连接被拒绝：请检查端口与防火墙"}
	case errors.Is(err, cloakdriver.ErrManagerTimeout):
		return cloakTestResponse{Category: cloakCatTimeout, Message: "请求超时：CloakBrowser-Manager 未在限定时间内响应"}
	case errors.Is(err, cloakdriver.ErrManagerProtocolError):
		return cloakTestResponse{Category: cloakCatProtocolError, Message: "协议错误：响应不是有效的 JSON 或格式不符"}
	default:
		return cloakTestResponse{Category: cloakCatUnknown, Message: "未知错误，请查看后端日志"}
	}
}
