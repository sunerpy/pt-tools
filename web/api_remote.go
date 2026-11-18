package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/remote"
)

// 「系统 → 远程访问」的接口（路线图 M15），只认 session。错误是一句中文，网页直接显示。

func (s *Server) remoteHost(w http.ResponseWriter) (*remote.Host, bool) {
	if s.remote == nil {
		http.Error(w, "远程访问没有初始化", http.StatusServiceUnavailable)
		return nil, false
	}
	return s.remote, true
}

func writeRemoteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, remote.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, remote.ErrDeviceNotFound):
		status = http.StatusNotFound
	case errors.Is(err, remote.ErrDisabled), errors.Is(err, remote.ErrDeviceRevoked), errors.Is(err, remote.ErrDeviceActive):
		status = http.StatusConflict
	case errors.Is(err, remote.ErrNoCipher):
		status = http.StatusServiceUnavailable
	default:
		global.GetSlogger().Warnf("[远程访问] 接口出错: %v", err)
	}
	http.Error(w, err.Error(), status)
}

func (s *Server) apiRemoteGet(w http.ResponseWriter, r *http.Request) {
	h, ok := s.remoteHost(w)
	if !ok {
		return
	}
	ov, err := h.Overview(r.Context())
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, ov)
}

func (s *Server) apiRemotePut(w http.ResponseWriter, r *http.Request) {
	h, ok := s.remoteHost(w)
	if !ok {
		return
	}
	var in remote.Settings
	if !decodeStrictBody(w, r, &in) {
		return
	}
	ov, err := h.UpdateSettings(r.Context(), in)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, ov)
}

// apiRemoteRotate 换一套主机密钥：所有设备撤销、要重新配对。
func (s *Server) apiRemoteRotate(w http.ResponseWriter, r *http.Request) {
	h, ok := s.remoteHost(w)
	if !ok {
		return
	}
	ov, err := h.RotateKeys(r.Context())
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, ov)
}

// RemotePairingInput 是添加设备的请求：权限（默认完全控制），以及这次链接里的直连地址（不填时用设置里的）。
type RemotePairingInput struct {
	Scopes    []string `json:"scopes"`
	DirectURL string   `json:"direct_url"`
}

// RemotePairingView 是新开的配对窗口：链接与二维码只在这里给一次。
type RemotePairingView struct {
	remote.PairingTicket
	// QRSVG 是链接的二维码（SVG 文本）
	QRSVG string `json:"qr_svg"`
}

func (s *Server) apiRemotePairingStart(w http.ResponseWriter, r *http.Request) {
	h, ok := s.remoteHost(w)
	if !ok {
		return
	}
	var in RemotePairingInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	if len(in.Scopes) == 0 {
		in.Scopes = remote.ScopesFull
	}
	ticket, err := h.StartPairing(r.Context(), in.Scopes, in.DirectURL)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	svg, err := qrSVG(ticket.Link)
	if err != nil {
		h.CancelPairing()
		http.Error(w, "生成二维码失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 链接里有一次性的配对密钥：不缓存
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, RemotePairingView{PairingTicket: ticket, QRSVG: svg})
}

func (s *Server) apiRemotePairingStatus(w http.ResponseWriter, _ *http.Request) {
	h, ok := s.remoteHost(w)
	if !ok {
		return
	}
	writeJSON(w, h.PairingStatus())
}

func (s *Server) apiRemotePairingCancel(w http.ResponseWriter, _ *http.Request) {
	h, ok := s.remoteHost(w)
	if !ok {
		return
	}
	h.CancelPairing()
	writeJSON(w, h.PairingStatus())
}

func (s *Server) apiRemoteDevices(w http.ResponseWriter, r *http.Request) {
	h, ok := s.remoteHost(w)
	if !ok {
		return
	}
	list, err := h.Devices(r.Context())
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, list)
}

// remoteDeviceID 读路径里的设备编号，不对时回 400。
func remoteDeviceID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		http.Error(w, "设备编号不对", http.StatusBadRequest)
		return 0, false
	}
	return uint(id), true
}

// RemoteDeviceInput 是改设备的请求：不带的字段不改。
type RemoteDeviceInput struct {
	Name   *string  `json:"name"`
	Scopes []string `json:"scopes"`
}

func (s *Server) apiRemoteDeviceUpdate(w http.ResponseWriter, r *http.Request) {
	h, ok := s.remoteHost(w)
	if !ok {
		return
	}
	id, ok := remoteDeviceID(w, r)
	if !ok {
		return
	}
	var in RemoteDeviceInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	d, err := h.UpdateDevice(r.Context(), id, in.Name, in.Scopes)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, d)
}

func (s *Server) apiRemoteDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	h, ok := s.remoteHost(w)
	if !ok {
		return
	}
	id, ok := remoteDeviceID(w, r)
	if !ok {
		return
	}
	d, err := h.RevokeDevice(r.Context(), id)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, d)
}

func (s *Server) apiRemoteDeviceDelete(w http.ResponseWriter, r *http.Request) {
	h, ok := s.remoteHost(w)
	if !ok {
		return
	}
	id, ok := remoteDeviceID(w, r)
	if !ok {
		return
	}
	if err := h.DeleteDevice(r.Context(), id); err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
