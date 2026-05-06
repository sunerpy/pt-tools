package mcp

import (
	"context"
	"errors"
	"net/http"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func ServeStdio(ctx context.Context, srv *mcpsdk.Server) error {
	if srv == nil {
		return errors.New("nil mcp server")
	}
	return srv.Run(ctx, &mcpsdk.StdioTransport{})
}

func ServeHTTP(ctx context.Context, srv *mcpsdk.Server, addr string) error {
	if srv == nil {
		return errors.New("nil mcp server")
	}
	if addr == "" {
		return errors.New("empty listen address")
	}

	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return srv
	}, nil)

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		errCh <- httpSrv.Shutdown(shutdownCtx)
	}()

	err := httpSrv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		select {
		case shutdownErr := <-errCh:
			if shutdownErr != nil {
				return shutdownErr
			}
		default:
		}
		return nil
	}
	return err
}
