package web

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"rsc.io/qr"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/remote"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// 远程访问（路线图 M15）：手机 App 配对以后经直连 WebSocket 或 relay 调用 App API v1。
// 协议在 internal/remote；这里是三件事：直连入口 /remote/v1/stream，设备会话里请求的分发（主体是这台设备），
// 以及「系统 → 远程访问」的接口（只认 session）。

// SetRemoteHost 接上远程访问的主机端（cmd 构造）。没接时直连入口 404，设置接口回 503。
func (s *Server) SetRemoteHost(h *remote.Host) { s.remote = h }

// remoteDeviceKey 是隧道分发器放进请求 context 的设备主体。键没有导出，HTTP 请求伪造不了。
type remoteDeviceKey struct{}

// remotePrincipal 取隧道分发器放进来的设备主体，不是隧道里的请求时返回 nil。
func remotePrincipal(r *http.Request) *middleware.Principal {
	p, _ := r.Context().Value(remoteDeviceKey{}).(*middleware.Principal)
	return p
}

// RemoteDispatcher 是设备会话里请求的分发器：只挂 App API v1 的路由（不经主 mux，别的接口在这里根本不存在）。
// 主体是这台设备：权限范围是配对时（或之后改的）app:read / app:write，写请求按 remote_device 记审计。
func (s *Server) RemoteDispatcher() remote.Dispatcher {
	mux := http.NewServeMux()
	s.registerAppV1Routes(mux)
	return remoteDispatcher{mux: mux}
}

type remoteDispatcher struct{ mux http.Handler }

func (d remoteDispatcher) ServeRemote(w http.ResponseWriter, r *http.Request, peer remote.Peer) {
	p := &middleware.Principal{
		Kind: middleware.KindRemoteDevice, ID: strconv.FormatUint(uint64(peer.Device.ID), 10), Name: peer.Device.Name,
		Scopes: slices.Clone(peer.Device.Scopes),
	}
	// 隧道已经去掉了 Cookie 与 Authorization，这里再删一遍，主体只认上面这个
	r.Header.Del("Cookie")
	r.Header.Del("Authorization")
	d.mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), remoteDeviceKey{}, p)))
}

// RemoteAudit 记下配对的结果（主机端的 Config.Audit）：通道是 remote_device，用户是设备编号（密钥不对时是 0）。
func (s *Server) RemoteAudit(ctx context.Context, deviceID uint, command, result string) {
	if s.appAudit == nil {
		return
	}
	e := app.AuditEntry{ChannelType: middleware.KindRemoteDevice, ChannelUserID: strconv.FormatUint(uint64(deviceID), 10), Command: command, Result: result}
	if err := s.appAudit.Record(ctx, e); err != nil {
		global.GetSlogger().Warnf("[远程访问] 记审计失败: %v", err)
	}
}

func (s *Server) registerRemoteRoutes(mux *http.ServeMux) {
	mux.HandleFunc(remote.StreamPath, func(w http.ResponseWriter, r *http.Request) {
		if s.remote == nil {
			http.NotFound(w, r)
			return
		}
		s.remote.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /api/remote", s.auth(s.apiRemoteGet))
	mux.HandleFunc("PUT /api/remote", s.auth(s.apiRemotePut))
	mux.HandleFunc("POST /api/remote/keys/rotate", s.auth(s.apiRemoteRotate))
	mux.HandleFunc("POST /api/remote/pairings", s.auth(s.apiRemotePairingStart))
	mux.HandleFunc("GET /api/remote/pairings/current", s.auth(s.apiRemotePairingStatus))
	mux.HandleFunc("DELETE /api/remote/pairings/current", s.auth(s.apiRemotePairingCancel))
	mux.HandleFunc("GET /api/remote/devices", s.auth(s.apiRemoteDevices))
	mux.HandleFunc("PUT /api/remote/devices/{id}", s.auth(s.apiRemoteDeviceUpdate))
	mux.HandleFunc("POST /api/remote/devices/{id}/revoke", s.auth(s.apiRemoteDeviceRevoke))
	mux.HandleFunc("DELETE /api/remote/devices/{id}", s.auth(s.apiRemoteDeviceDelete))
}

// qrSVG 把文本编成二维码（纠错级别 M），写成 SVG：深色模块拼成一条路径，四周留 4 个模块的空白。
func qrSVG(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	const quiet = 4
	n := code.Size + 2*quiet
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, n, n)
	b.WriteString(`<rect width="100%" height="100%" fill="#fff"/><path fill="#000" d="`)
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String(), nil
}
