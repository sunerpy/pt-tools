package models

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

// 每个以 Encrypted 结尾的字符串字段都是 AES-GCM 密文，都要在 EncryptedColumns 里：清单漏了的列，
// 启动时的密钥检查和备份校验都看不到（新生成的密钥解不开它们，却照常启动或照常恢复）。
func TestEncryptedColumnsCoverEveryEncryptedField(t *testing.T) {
	listed := map[EncryptedColumn]bool{}
	for _, c := range EncryptedColumns {
		assert.False(t, listed[c], "%s.%s 在清单里出现了两次", c.Table, c.Column)
		listed[c] = true
	}
	cache := &sync.Map{}
	found := 0
	for _, m := range AllModels() {
		s, err := schema.Parse(m, cache, schema.NamingStrategy{})
		require.NoError(t, err)
		for _, f := range s.Fields {
			if f.FieldType.Kind() != reflect.String || !strings.HasSuffix(f.Name, "Encrypted") {
				continue
			}
			found++
			assert.True(t, listed[EncryptedColumn{s.Table, f.DBName}], "%s.%s（%s.%s）不在 EncryptedColumns 里", s.Table, f.DBName, s.Name, f.Name)
		}
	}
	assert.Greater(t, found, 0, "一个 Encrypted 字段都没找到，AllModels 或判断条件不对")
}
