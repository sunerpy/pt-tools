// Command testhost 是手机 App（apps/mobile）互通测试用的主机：真正的 internal/remote 主机端（直连入口与 relay 客户端），
// App API 换成固定的回应，加一台进程内的 Go relay（relayserver）。地址写进 -info 指定的 JSON 文件，Dart 的 test/interop 读它：
//
//	go run ./internal/remote/testhost -info /tmp/testhost.json
//	PTT_TESTHOST=/tmp/testhost.json flutter test test/interop
//
// 控制接口（只监听本机）：POST /control/pair?scopes=full|read 开配对窗口并返回链接，GET /control/devices，
// POST /control/devices/{id}/revoke，POST /control/devices/{id}/scopes?scopes=read|full。只在测试与 CI 里用。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/remote"
	"github.com/sunerpy/pt-tools/internal/remote/relayserver"
	"github.com/sunerpy/pt-tools/models"
)

// plainCipher 是测试主机的主机密钥加解密（不保密，只是走一遍存库的路径）。
type plainCipher struct{}

func (plainCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (plainCipher) Decrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("不是密文")
	}
	return strings.TrimPrefix(c, "enc:"), nil
}

// appAPI 是固定的 App API：/meta 按设备回主体与权限，/echo 原样回请求体，别的回路径、方法、设备与连接方式。
type appAPI struct{}

func (appAPI) ServeRemote(w http.ResponseWriter, r *http.Request, p remote.Peer) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/app/v1/meta":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "pt-tools", "version": "testhost", "remote_api_level": 1, "features": []string{"overview"},
			"principal": map[string]any{"kind": "remote_device", "name": p.Device.Name, "scopes": p.Device.Scopes},
		})
	case "/api/app/v1/echo":
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.Copy(w, r.Body)
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"path": r.URL.RequestURI(), "method": r.Method, "device": p.Device.Name, "via": p.Via})
	}
}

func main() {
	info := flag.String("info", "testhost.json", "写地址的 JSON 文件")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *info); err != nil {
		log.Fatal(err)
	}
}

func listen(ctx context.Context) (net.Listener, string, error) {
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	return l, l.Addr().String(), nil
}

// run 起 relay 与主机，写好地址文件，一直运行到 ctx 结束。
func run(ctx context.Context, infoPath string) error {
	// relay
	rl, raddr, err := listen(ctx)
	if err != nil {
		return err
	}
	relayURL := "ws://" + raddr
	relay, err := relayserver.New(relayserver.Config{PublicURL: relayURL, MaxConnPerIPPerMin: -1, Version: "testhost"})
	if err != nil {
		return err
	}
	defer relay.Close()
	go func() { _ = (&http.Server{Handler: relay.Handler(), ReadHeaderTimeout: 10 * time.Second}).Serve(rl) }()

	// 主机
	dir, err := os.MkdirTemp("", "ptt-testhost-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(dir, "remote.db")), &gorm.Config{})
	if err != nil {
		return err
	}
	if sqlDB, derr := db.DB(); derr == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err = db.AutoMigrate(&models.RemoteSetting{}, &models.RemoteDevice{}); err != nil {
		return err
	}
	host := remote.New(remote.Config{Store: remote.NewStore(db, plainCipher{}), Dispatcher: appAPI{}, Version: "testhost"})
	defer host.Close()

	hl, haddr, err := listen(ctx)
	if err != nil {
		return err
	}
	direct := "http://" + haddr
	if _, err = host.UpdateSettings(ctx, remote.Settings{Enabled: true, Relays: []string{relayURL}, DirectURL: direct}); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/remote/v1/stream", host)
	registerControl(mux, host)
	go func() { _ = (&http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}).Serve(hl) }()

	// 等主机经 relay 上线，再写地址文件（测试读到文件就可以开始）
	deadline := time.Now().Add(20 * time.Second)
	for {
		ov, oerr := host.Overview(ctx)
		if oerr == nil && len(ov.RelayStatus) == 1 && ov.RelayStatus[0].State == remote.RelayOnline {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("主机没有经 relay 上线")
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, err := json.Marshal(map[string]string{"direct": direct, "relay": relayURL, "control": direct + "/control"})
	if err != nil {
		return err
	}
	tmp := infoPath + ".tmp"
	if err = os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmp, infoPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "testhost 直连 %s，relay %s，地址写在 %s\n", direct, relayURL, infoPath)
	<-ctx.Done()
	_ = os.Remove(infoPath)
	return nil
}

func scopesOf(r *http.Request) []string {
	if r.URL.Query().Get("scopes") == "read" {
		return remote.ScopesRead
	}
	return remote.ScopesFull
}

func registerControl(mux *http.ServeMux, host *remote.Host) {
	reply := func(w http.ResponseWriter, v any, err error) {
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	}
	id := func(r *http.Request) (uint, error) {
		n, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
		return uint(n), err
	}
	mux.HandleFunc("POST /control/pair", func(w http.ResponseWriter, r *http.Request) {
		t, err := host.StartPairing(r.Context(), scopesOf(r), "")
		reply(w, t, err)
	})
	mux.HandleFunc("GET /control/devices", func(w http.ResponseWriter, r *http.Request) {
		list, err := host.Devices(r.Context())
		reply(w, list, err)
	})
	mux.HandleFunc("POST /control/devices/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		n, err := id(r)
		if err != nil {
			reply(w, nil, err)
			return
		}
		d, err := host.RevokeDevice(r.Context(), n)
		reply(w, d, err)
	})
	mux.HandleFunc("POST /control/devices/{id}/scopes", func(w http.ResponseWriter, r *http.Request) {
		n, err := id(r)
		if err != nil {
			reply(w, nil, err)
			return
		}
		d, err := host.UpdateDevice(r.Context(), n, nil, scopesOf(r))
		reply(w, d, err)
	})
}
