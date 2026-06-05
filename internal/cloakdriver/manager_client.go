package cloakdriver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultManagerTimeout = 10 * time.Second

type httpManagerClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewManagerClient builds a ManagerClient bound to the given Manager base URL.
// A zero timeout uses defaultManagerTimeout (10s).
func NewManagerClient(baseURL, token string, timeout time.Duration) ManagerClient {
	if timeout <= 0 {
		timeout = defaultManagerTimeout
	}
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
	}
	return &httpManagerClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}
}

func (c *httpManagerClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	full := c.baseURL + path
	if _, err := url.Parse(full); err != nil {
		return nil, fmt.Errorf("%w: invalid url: %v", ErrManagerProtocolError, err)
	}
	req, err := http.NewRequestWithContext(ctx, method, full, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManagerProtocolError, err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (c *httpManagerClient) do(req *http.Request) (status int, body []byte, err error) {
	resp, transportErr := c.httpClient.Do(req)
	if transportErr != nil {
		classified := classifyHTTPError(nil, transportErr)
		sLogger().Debugw("manager_request_transport_error",
			"method", req.Method, "path", req.URL.Path, "err", classified)
		return 0, nil, classified
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp.StatusCode, nil, classifyHTTPError(resp, readErr)
	}
	if classified := classifyHTTPError(resp, nil); classified != nil {
		sLogger().Debugw("manager_request_status_error",
			"method", req.Method, "path", req.URL.Path, "status", resp.StatusCode, "err", classified)
		return resp.StatusCode, body, classified
	}
	return resp.StatusCode, body, nil
}

// classifyHTTPError maps transport errors and HTTP status codes to sentinels (R19).
func classifyHTTPError(resp *http.Response, transportErr error) error {
	if transportErr != nil {
		if errors.Is(transportErr, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %v", ErrManagerTimeout, transportErr)
		}
		var dnsErr *net.DNSError
		if errors.As(transportErr, &dnsErr) {
			return fmt.Errorf("%w: %v", ErrManagerDNSFailed, transportErr)
		}
		var urlErr *url.Error
		if errors.As(transportErr, &urlErr) {
			if urlErr.Timeout() {
				return fmt.Errorf("%w: %v", ErrManagerTimeout, transportErr)
			}
			var inner *net.DNSError
			if errors.As(urlErr.Err, &inner) {
				return fmt.Errorf("%w: %v", ErrManagerDNSFailed, transportErr)
			}
			var opErr *net.OpError
			if errors.As(urlErr.Err, &opErr) {
				if opErr.Op == "dial" {
					msg := transportErr.Error()
					if strings.Contains(msg, "no such host") {
						return fmt.Errorf("%w: %v", ErrManagerDNSFailed, transportErr)
					}
					if strings.Contains(msg, "refused") {
						return fmt.Errorf("%w: %v", ErrManagerConnRefused, transportErr)
					}
				}
			}
		}
		var opErr *net.OpError
		if errors.As(transportErr, &opErr) {
			if opErr.Op == "dial" && strings.Contains(transportErr.Error(), "refused") {
				return fmt.Errorf("%w: %v", ErrManagerConnRefused, transportErr)
			}
		}
		if os.IsTimeout(transportErr) {
			return fmt.Errorf("%w: %v", ErrManagerTimeout, transportErr)
		}
		return fmt.Errorf("%w: %v", ErrManagerProtocolError, transportErr)
	}
	if resp == nil {
		return nil
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrManagerAuthFailed
	case resp.StatusCode == http.StatusNotFound:
		return ErrManagerNotFound
	case resp.StatusCode >= 500:
		return ErrManagerServerError
	case resp.StatusCode >= 400:
		return fmt.Errorf("%w: status=%d", ErrManagerProtocolError, resp.StatusCode)
	}
	return nil
}

func (c *httpManagerClient) LaunchProfile(ctx context.Context, profileID string) (*ProfileLaunchResult, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/profiles/"+profileID+"/launch", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	_, body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	var lr ProfileLaunchResult
	if uErr := json.Unmarshal(body, &lr); uErr != nil {
		return nil, fmt.Errorf("%w: launch response: %v", ErrManagerProtocolError, uErr)
	}
	return &lr, nil
}

func (c *httpManagerClient) GetProfileStatus(ctx context.Context, profileID string) (*ProfileStatus, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/profiles/"+profileID+"/status", nil)
	if err != nil {
		return nil, err
	}
	_, body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	var ps ProfileStatus
	if uErr := json.Unmarshal(body, &ps); uErr != nil {
		return nil, fmt.Errorf("%w: status response: %v", ErrManagerProtocolError, uErr)
	}
	return &ps, nil
}

func (c *httpManagerClient) StopProfile(ctx context.Context, profileID string) error {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/profiles/"+profileID+"/stop", nil)
	if err != nil {
		return err
	}
	_, _, err = c.do(req)
	return err
}

// DeleteProfile is idempotent (R27): a 404 from the second call is treated as success.
func (c *httpManagerClient) DeleteProfile(ctx context.Context, profileID string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/api/profiles/"+profileID, nil)
	if err != nil {
		return err
	}
	_, _, err = c.do(req)
	if errors.Is(err, ErrManagerNotFound) {
		return nil
	}
	return err
}

func (c *httpManagerClient) ManagerStatus(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/status", nil)
	if err != nil {
		return err
	}
	_, _, err = c.do(req)
	return err
}

func (c *httpManagerClient) ManagerStatusFull(ctx context.Context) (*ManagerStatusInfo, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/status", nil)
	if err != nil {
		return nil, err
	}
	_, body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	var info ManagerStatusInfo
	if uErr := json.Unmarshal(body, &info); uErr != nil {
		return nil, fmt.Errorf("%w: status response: %v", ErrManagerProtocolError, uErr)
	}
	return &info, nil
}
