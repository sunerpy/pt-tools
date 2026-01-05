package mocks

import (
	"testing"

	"github.com/gocolly/colly"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestMockSiteParser_Expectations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := NewMockSiteParser(ctrl)
	e := &colly.HTMLElement{}
	info := &models.PHPTorrentInfo{}
	m.EXPECT().ParseTitleAndID(e, info)
	m.EXPECT().ParseDiscount(e, info)
	m.EXPECT().ParseHR(e, info)
	m.EXPECT().ParseTorrentSizeMB(e, info)
	m.ParseTitleAndID(e, info)
	m.ParseDiscount(e, info)
	m.ParseHR(e, info)
	m.ParseTorrentSizeMB(e, info)
	require.True(t, true)
}
