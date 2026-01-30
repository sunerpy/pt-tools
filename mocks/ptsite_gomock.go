package mocks

import (
	"context"
	"reflect"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/mmcdole/gofeed"

	"github.com/sunerpy/pt-tools/models"
)

type (
	MockPTSiteInter[T models.ResType] struct {
		ctrl     *gomock.Controller
		recorder *MockPTSiteInterMockRecorder[T]
	}
	MockPTSiteInterMockRecorder[T models.ResType] struct{ mock *MockPTSiteInter[T] }
)

func NewMockPTSiteInter[T models.ResType](ctrl *gomock.Controller) *MockPTSiteInter[T] {
	m := &MockPTSiteInter[T]{ctrl: ctrl}
	m.recorder = &MockPTSiteInterMockRecorder[T]{mock: m}
	return m
}
func (m *MockPTSiteInter[T]) EXPECT() *MockPTSiteInterMockRecorder[T] { return m.recorder }
func (m *MockPTSiteInter[T]) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[T], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTorrentDetails", item)
	var r0 *models.APIResponse[T]
	if ret[0] != nil {
		r0 = ret[0].(*models.APIResponse[T])
	}
	var r1 error
	if ret[1] != nil {
		r1 = ret[1].(error)
	}
	return r0, r1
}

func (r *MockPTSiteInterMockRecorder[T]) GetTorrentDetails(item any) *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "GetTorrentDetails", reflect.TypeOf((*MockPTSiteInter[T])(nil).GetTorrentDetails), item)
}

func (m *MockPTSiteInter[T]) IsEnabled() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsEnabled")
	return ret[0].(bool)
}

func (r *MockPTSiteInterMockRecorder[T]) IsEnabled() *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "IsEnabled", reflect.TypeOf((*MockPTSiteInter[T])(nil).IsEnabled))
}

func (m *MockPTSiteInter[T]) DownloadTorrent(url, title, dir string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DownloadTorrent", url, title, dir)
	var r0 string
	if ret[0] != nil {
		r0 = ret[0].(string)
	}
	var r1 error
	if ret[1] != nil {
		r1 = ret[1].(error)
	}
	return r0, r1
}

func (r *MockPTSiteInterMockRecorder[T]) DownloadTorrent(url, title, dir any) *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "DownloadTorrent", reflect.TypeOf((*MockPTSiteInter[T])(nil).DownloadTorrent), url, title, dir)
}

func (m *MockPTSiteInter[T]) MaxRetries() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MaxRetries")
	return ret[0].(int)
}

func (r *MockPTSiteInterMockRecorder[T]) MaxRetries() *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "MaxRetries", reflect.TypeOf((*MockPTSiteInter[T])(nil).MaxRetries))
}

func (m *MockPTSiteInter[T]) RetryDelay() time.Duration {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RetryDelay")
	return ret[0].(time.Duration)
}

func (r *MockPTSiteInterMockRecorder[T]) RetryDelay() *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "RetryDelay", reflect.TypeOf((*MockPTSiteInter[T])(nil).RetryDelay))
}

func (m *MockPTSiteInter[T]) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SendTorrentToDownloader", ctx, rssCfg)
	var r0 error
	if ret[0] != nil {
		r0 = ret[0].(error)
	}
	return r0
}

func (r *MockPTSiteInterMockRecorder[T]) SendTorrentToDownloader(ctx, rssCfg any) *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "SendTorrentToDownloader", reflect.TypeOf((*MockPTSiteInter[T])(nil).SendTorrentToDownloader), ctx, rssCfg)
}

func (m *MockPTSiteInter[T]) Context() context.Context {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Context")
	return ret[0].(context.Context)
}

func (r *MockPTSiteInterMockRecorder[T]) Context() *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "Context", reflect.TypeOf((*MockPTSiteInter[T])(nil).Context))
}
