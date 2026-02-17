package mocks

import (
	"context"
	"reflect"

	"github.com/golang/mock/gomock"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// MockDownloader is a mock of Downloader interface
type MockDownloader struct {
	ctrl     *gomock.Controller
	recorder *MockDownloaderMockRecorder
}

// MockDownloaderMockRecorder is the mock recorder for MockDownloader
type MockDownloaderMockRecorder struct {
	mock *MockDownloader
}

// NewMockDownloader creates a new mock instance
func NewMockDownloader(ctrl *gomock.Controller) *MockDownloader {
	mock := &MockDownloader{ctrl: ctrl}
	mock.recorder = &MockDownloaderMockRecorder{mock: mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockDownloader) EXPECT() *MockDownloaderMockRecorder {
	return m.recorder
}

// Authenticate mocks base method
func (m *MockDownloader) Authenticate() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Authenticate")
	ret0, _ := ret[0].(error)
	return ret0
}

// Authenticate indicates an expected call of Authenticate
func (mr *MockDownloaderMockRecorder) Authenticate() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Authenticate", reflect.TypeOf((*MockDownloader)(nil).Authenticate))
}

// Ping mocks base method
func (m *MockDownloader) Ping() (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Ping")
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Ping indicates an expected call of Ping
func (mr *MockDownloaderMockRecorder) Ping() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Ping", reflect.TypeOf((*MockDownloader)(nil).Ping))
}

// GetClientVersion mocks base method
func (m *MockDownloader) GetClientVersion() (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClientVersion")
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClientVersion indicates an expected call of GetClientVersion
func (mr *MockDownloaderMockRecorder) GetClientVersion() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClientVersion", reflect.TypeOf((*MockDownloader)(nil).GetClientVersion))
}

// GetClientStatus mocks base method
func (m *MockDownloader) GetClientStatus() (downloader.ClientStatus, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClientStatus")
	ret0, _ := ret[0].(downloader.ClientStatus)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClientStatus indicates an expected call of GetClientStatus
func (mr *MockDownloaderMockRecorder) GetClientStatus() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClientStatus", reflect.TypeOf((*MockDownloader)(nil).GetClientStatus))
}

// GetClientFreeSpace mocks base method
func (m *MockDownloader) GetClientFreeSpace(ctx context.Context) (int64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClientFreeSpace", ctx)
	ret0, _ := ret[0].(int64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClientFreeSpace indicates an expected call of GetClientFreeSpace
func (mr *MockDownloaderMockRecorder) GetClientFreeSpace(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClientFreeSpace", reflect.TypeOf((*MockDownloader)(nil).GetClientFreeSpace), ctx)
}

// GetAllTorrents mocks base method
func (m *MockDownloader) GetAllTorrents() ([]downloader.Torrent, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAllTorrents")
	ret0, _ := ret[0].([]downloader.Torrent)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAllTorrents indicates an expected call of GetAllTorrents
func (mr *MockDownloaderMockRecorder) GetAllTorrents() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAllTorrents", reflect.TypeOf((*MockDownloader)(nil).GetAllTorrents))
}

// GetTorrentsBy mocks base method
func (m *MockDownloader) GetTorrentsBy(filter downloader.TorrentFilter) ([]downloader.Torrent, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTorrentsBy", filter)
	ret0, _ := ret[0].([]downloader.Torrent)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTorrentsBy indicates an expected call of GetTorrentsBy
func (mr *MockDownloaderMockRecorder) GetTorrentsBy(filter any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTorrentsBy", reflect.TypeOf((*MockDownloader)(nil).GetTorrentsBy), filter)
}

// GetTorrent mocks base method
func (m *MockDownloader) GetTorrent(id string) (downloader.Torrent, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTorrent", id)
	ret0, _ := ret[0].(downloader.Torrent)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTorrent indicates an expected call of GetTorrent
func (mr *MockDownloaderMockRecorder) GetTorrent(id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTorrent", reflect.TypeOf((*MockDownloader)(nil).GetTorrent), id)
}

// AddTorrentEx mocks base method
func (m *MockDownloader) AddTorrentEx(url string, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddTorrentEx", url, opt)
	ret0, _ := ret[0].(downloader.AddTorrentResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// AddTorrentEx indicates an expected call of AddTorrentEx
func (mr *MockDownloaderMockRecorder) AddTorrentEx(url, opt any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddTorrentEx", reflect.TypeOf((*MockDownloader)(nil).AddTorrentEx), url, opt)
}

// AddTorrentFileEx mocks base method
func (m *MockDownloader) AddTorrentFileEx(fileData []byte, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddTorrentFileEx", fileData, opt)
	ret0, _ := ret[0].(downloader.AddTorrentResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// AddTorrentFileEx indicates an expected call of AddTorrentFileEx
func (mr *MockDownloaderMockRecorder) AddTorrentFileEx(fileData, opt any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddTorrentFileEx", reflect.TypeOf((*MockDownloader)(nil).AddTorrentFileEx), fileData, opt)
}

// PauseTorrent mocks base method
func (m *MockDownloader) PauseTorrent(id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PauseTorrent", id)
	ret0, _ := ret[0].(error)
	return ret0
}

// PauseTorrent indicates an expected call of PauseTorrent
func (mr *MockDownloaderMockRecorder) PauseTorrent(id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PauseTorrent", reflect.TypeOf((*MockDownloader)(nil).PauseTorrent), id)
}

// ResumeTorrent mocks base method
func (m *MockDownloader) ResumeTorrent(id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ResumeTorrent", id)
	ret0, _ := ret[0].(error)
	return ret0
}

// ResumeTorrent indicates an expected call of ResumeTorrent
func (mr *MockDownloaderMockRecorder) ResumeTorrent(id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ResumeTorrent", reflect.TypeOf((*MockDownloader)(nil).ResumeTorrent), id)
}

// RemoveTorrent mocks base method
func (m *MockDownloader) RemoveTorrent(id string, removeData bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RemoveTorrent", id, removeData)
	ret0, _ := ret[0].(error)
	return ret0
}

// RemoveTorrent indicates an expected call of RemoveTorrent
func (mr *MockDownloaderMockRecorder) RemoveTorrent(id, removeData any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoveTorrent", reflect.TypeOf((*MockDownloader)(nil).RemoveTorrent), id, removeData)
}

// GetClientPaths mocks base method
func (m *MockDownloader) GetClientPaths() ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClientPaths")
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClientPaths indicates an expected call of GetClientPaths
func (mr *MockDownloaderMockRecorder) GetClientPaths() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClientPaths", reflect.TypeOf((*MockDownloader)(nil).GetClientPaths))
}

// GetClientLabels mocks base method
func (m *MockDownloader) GetClientLabels() ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClientLabels")
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClientLabels indicates an expected call of GetClientLabels
func (mr *MockDownloaderMockRecorder) GetClientLabels() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClientLabels", reflect.TypeOf((*MockDownloader)(nil).GetClientLabels))
}

// GetType mocks base method
func (m *MockDownloader) GetType() downloader.DownloaderType {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetType")
	ret0, _ := ret[0].(downloader.DownloaderType)
	return ret0
}

// GetType indicates an expected call of GetType
func (mr *MockDownloaderMockRecorder) GetType() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetType", reflect.TypeOf((*MockDownloader)(nil).GetType))
}

// GetName mocks base method
func (m *MockDownloader) GetName() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetName")
	ret0, _ := ret[0].(string)
	return ret0
}

// GetName indicates an expected call of GetName
func (mr *MockDownloaderMockRecorder) GetName() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetName", reflect.TypeOf((*MockDownloader)(nil).GetName))
}

// IsHealthy mocks base method
func (m *MockDownloader) IsHealthy() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsHealthy")
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsHealthy indicates an expected call of IsHealthy
func (mr *MockDownloaderMockRecorder) IsHealthy() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsHealthy", reflect.TypeOf((*MockDownloader)(nil).IsHealthy))
}

// Close mocks base method
func (m *MockDownloader) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close
func (mr *MockDownloaderMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockDownloader)(nil).Close))
}

// ============ 以下为向后兼容的旧接口 ============

// AddTorrent mocks base method
func (m *MockDownloader) AddTorrent(fileData []byte, category, tags string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddTorrent", fileData, category, tags)
	ret0, _ := ret[0].(error)
	return ret0
}

// AddTorrent indicates an expected call of AddTorrent
func (mr *MockDownloaderMockRecorder) AddTorrent(fileData, category, tags any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddTorrent", reflect.TypeOf((*MockDownloader)(nil).AddTorrent), fileData, category, tags)
}

// AddTorrentWithPath mocks base method
func (m *MockDownloader) AddTorrentWithPath(fileData []byte, category, tags, downloadPath string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddTorrentWithPath", fileData, category, tags, downloadPath)
	ret0, _ := ret[0].(error)
	return ret0
}

// AddTorrentWithPath indicates an expected call of AddTorrentWithPath
func (mr *MockDownloaderMockRecorder) AddTorrentWithPath(fileData, category, tags, downloadPath any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddTorrentWithPath", reflect.TypeOf((*MockDownloader)(nil).AddTorrentWithPath), fileData, category, tags, downloadPath)
}

// CheckTorrentExists mocks base method
func (m *MockDownloader) CheckTorrentExists(torrentHash string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CheckTorrentExists", torrentHash)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CheckTorrentExists indicates an expected call of CheckTorrentExists
func (mr *MockDownloaderMockRecorder) CheckTorrentExists(torrentHash any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CheckTorrentExists", reflect.TypeOf((*MockDownloader)(nil).CheckTorrentExists), torrentHash)
}

// GetDiskSpace mocks base method
func (m *MockDownloader) GetDiskSpace(ctx context.Context) (int64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetDiskSpace", ctx)
	ret0, _ := ret[0].(int64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetDiskSpace indicates an expected call of GetDiskSpace
func (mr *MockDownloaderMockRecorder) GetDiskSpace(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetDiskSpace", reflect.TypeOf((*MockDownloader)(nil).GetDiskSpace), ctx)
}

// CanAddTorrent mocks base method
func (m *MockDownloader) CanAddTorrent(ctx context.Context, fileSize int64) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CanAddTorrent", ctx, fileSize)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CanAddTorrent indicates an expected call of CanAddTorrent
func (mr *MockDownloaderMockRecorder) CanAddTorrent(ctx, fileSize any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CanAddTorrent", reflect.TypeOf((*MockDownloader)(nil).CanAddTorrent), ctx, fileSize)
}

// ProcessSingleTorrentFile mocks base method
func (m *MockDownloader) ProcessSingleTorrentFile(ctx context.Context, filePath, category, tags string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ProcessSingleTorrentFile", ctx, filePath, category, tags)
	ret0, _ := ret[0].(error)
	return ret0
}

// ProcessSingleTorrentFile indicates an expected call of ProcessSingleTorrentFile
func (mr *MockDownloaderMockRecorder) ProcessSingleTorrentFile(ctx, filePath, category, tags any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ProcessSingleTorrentFile", reflect.TypeOf((*MockDownloader)(nil).ProcessSingleTorrentFile), ctx, filePath, category, tags)
}

// PauseTorrents mocks base method
func (m *MockDownloader) PauseTorrents(ids []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PauseTorrents", ids)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockDownloaderMockRecorder) PauseTorrents(ids any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PauseTorrents", reflect.TypeOf((*MockDownloader)(nil).PauseTorrents), ids)
}

// ResumeTorrents mocks base method
func (m *MockDownloader) ResumeTorrents(ids []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ResumeTorrents", ids)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockDownloaderMockRecorder) ResumeTorrents(ids any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ResumeTorrents", reflect.TypeOf((*MockDownloader)(nil).ResumeTorrents), ids)
}

// RemoveTorrents mocks base method
func (m *MockDownloader) RemoveTorrents(ids []string, removeData bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RemoveTorrents", ids, removeData)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockDownloaderMockRecorder) RemoveTorrents(ids, removeData any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoveTorrents", reflect.TypeOf((*MockDownloader)(nil).RemoveTorrents), ids, removeData)
}

// SetTorrentCategory mocks base method
func (m *MockDownloader) SetTorrentCategory(id, category string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetTorrentCategory", id, category)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockDownloaderMockRecorder) SetTorrentCategory(id, category any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetTorrentCategory", reflect.TypeOf((*MockDownloader)(nil).SetTorrentCategory), id, category)
}

// SetTorrentTags mocks base method
func (m *MockDownloader) SetTorrentTags(id, tags string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetTorrentTags", id, tags)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockDownloaderMockRecorder) SetTorrentTags(id, tags any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetTorrentTags", reflect.TypeOf((*MockDownloader)(nil).SetTorrentTags), id, tags)
}

// SetTorrentSavePath mocks base method
func (m *MockDownloader) SetTorrentSavePath(id, path string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetTorrentSavePath", id, path)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockDownloaderMockRecorder) SetTorrentSavePath(id, path any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetTorrentSavePath", reflect.TypeOf((*MockDownloader)(nil).SetTorrentSavePath), id, path)
}

// RecheckTorrent mocks base method
func (m *MockDownloader) RecheckTorrent(id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RecheckTorrent", id)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockDownloaderMockRecorder) RecheckTorrent(id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RecheckTorrent", reflect.TypeOf((*MockDownloader)(nil).RecheckTorrent), id)
}

// GetTorrentFiles mocks base method
func (m *MockDownloader) GetTorrentFiles(id string) ([]downloader.TorrentFile, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTorrentFiles", id)
	ret0, _ := ret[0].([]downloader.TorrentFile)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockDownloaderMockRecorder) GetTorrentFiles(id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTorrentFiles", reflect.TypeOf((*MockDownloader)(nil).GetTorrentFiles), id)
}

// GetTorrentTrackers mocks base method
func (m *MockDownloader) GetTorrentTrackers(id string) ([]downloader.TorrentTracker, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTorrentTrackers", id)
	ret0, _ := ret[0].([]downloader.TorrentTracker)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockDownloaderMockRecorder) GetTorrentTrackers(id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTorrentTrackers", reflect.TypeOf((*MockDownloader)(nil).GetTorrentTrackers), id)
}

// GetDiskInfo mocks base method
func (m *MockDownloader) GetDiskInfo() (downloader.DiskInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetDiskInfo")
	ret0, _ := ret[0].(downloader.DiskInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockDownloaderMockRecorder) GetDiskInfo() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetDiskInfo", reflect.TypeOf((*MockDownloader)(nil).GetDiskInfo))
}

// GetSpeedLimit mocks base method
func (m *MockDownloader) GetSpeedLimit() (downloader.SpeedLimit, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSpeedLimit")
	ret0, _ := ret[0].(downloader.SpeedLimit)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockDownloaderMockRecorder) GetSpeedLimit() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSpeedLimit", reflect.TypeOf((*MockDownloader)(nil).GetSpeedLimit))
}

// SetSpeedLimit mocks base method
func (m *MockDownloader) SetSpeedLimit(limit downloader.SpeedLimit) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetSpeedLimit", limit)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockDownloaderMockRecorder) SetSpeedLimit(limit any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetSpeedLimit", reflect.TypeOf((*MockDownloader)(nil).SetSpeedLimit), limit)
}
