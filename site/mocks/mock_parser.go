package mocks

import (
	"reflect"

	"github.com/gocolly/colly"
	"github.com/golang/mock/gomock"

	"github.com/sunerpy/pt-tools/models"
)

type MockSiteParser struct {
	ctrl     *gomock.Controller
	recorder *MockSiteParserMockRecorder
}
type MockSiteParserMockRecorder struct{ mock *MockSiteParser }

func NewMockSiteParser(ctrl *gomock.Controller) *MockSiteParser {
	m := &MockSiteParser{ctrl: ctrl}
	m.recorder = &MockSiteParserMockRecorder{mock: m}
	return m
}
func (m *MockSiteParser) EXPECT() *MockSiteParserMockRecorder { return m.recorder }
func (m *MockSiteParser) ParseTitleAndID(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ParseTitleAndID", e, info)
}

func (r *MockSiteParserMockRecorder) ParseTitleAndID(e, info any) *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "ParseTitleAndID", reflect.TypeOf((*MockSiteParser)(nil).ParseTitleAndID), e, info)
}

func (m *MockSiteParser) ParseDiscount(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ParseDiscount", e, info)
}

func (r *MockSiteParserMockRecorder) ParseDiscount(e, info any) *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "ParseDiscount", reflect.TypeOf((*MockSiteParser)(nil).ParseDiscount), e, info)
}

func (m *MockSiteParser) ParseHR(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ParseHR", e, info)
}

func (r *MockSiteParserMockRecorder) ParseHR(e, info any) *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "ParseHR", reflect.TypeOf((*MockSiteParser)(nil).ParseHR), e, info)
}

func (m *MockSiteParser) ParseTorrentSizeMB(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ParseTorrentSizeMB", e, info)
}

func (r *MockSiteParserMockRecorder) ParseTorrentSizeMB(e, info any) *gomock.Call {
	r.mock.ctrl.T.Helper()
	return r.mock.ctrl.RecordCallWithMethodType(r.mock, "ParseTorrentSizeMB", reflect.TypeOf((*MockSiteParser)(nil).ParseTorrentSizeMB), e, info)
}
