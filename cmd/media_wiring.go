package cmd

import (
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
)

// newMediaService 构造媒体识别服务：TMDB 的 API Key 与代理地址用 ConfigStore 的 Cookie 密钥加解密。
func newMediaService(store *core.ConfigStore) *recognize.Service {
	if global.GlobalDB == nil || store == nil {
		return nil
	}
	return recognize.New(recognize.Config{
		DB:           global.GlobalDB.DB,
		Cipher:       storeCipher{store: store},
		BaseURL:      qaTMDBBaseURL(),
		ImageBaseURL: qaTMDBImageURL(),
	})
}
