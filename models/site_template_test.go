package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestSiteTemplateTableName 测试表名
func TestSiteTemplateTableName(t *testing.T) {
	template := SiteTemplate{}
	assert.Equal(t, "site_templates", template.TableName())
}

// TestSiteTemplateToExport 测试导出转换
func TestSiteTemplateToExport(t *testing.T) {
	template := &SiteTemplate{
		ID:           1,
		Name:         "test-site",
		DisplayName:  "Test Site",
		BaseURL:      "https://example.com",
		AuthMethod:   "cookie",
		ParserConfig: `{"name": "test"}`,
		Description:  "Test description",
		Version:      "1.0.0",
		Author:       "Test Author",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	export := template.ToExport()

	assert.Equal(t, "test-site", export.Name)
	assert.Equal(t, "Test Site", export.DisplayName)
	assert.Equal(t, "https://example.com", export.BaseURL)
	assert.Equal(t, "cookie", export.AuthMethod)
	assert.Equal(t, "Test description", export.Description)
	assert.Equal(t, "1.0.0", export.Version)
	assert.Equal(t, "Test Author", export.Author)
	assert.Equal(t, json.RawMessage(`{"name": "test"}`), export.ParserConfig)
}

// TestSiteTemplateToExportEmptyParserConfig 测试空解析器配置的导出
func TestSiteTemplateToExportEmptyParserConfig(t *testing.T) {
	template := &SiteTemplate{
		Name:         "test-site",
		DisplayName:  "Test Site",
		BaseURL:      "https://example.com",
		AuthMethod:   "cookie",
		ParserConfig: "",
	}

	export := template.ToExport()

	assert.Nil(t, export.ParserConfig)
}

// TestSiteTemplateFromExport 测试从导出格式创建
func TestSiteTemplateFromExport(t *testing.T) {
	export := &SiteTemplateExport{
		Name:         "imported-site",
		DisplayName:  "Imported Site",
		BaseURL:      "https://imported.com",
		AuthMethod:   "api_key",
		ParserConfig: json.RawMessage(`{"parser": "config"}`),
		Description:  "Imported description",
		Version:      "2.0.0",
		Author:       "Import Author",
	}

	template := &SiteTemplate{}
	err := template.FromExport(export)

	assert.NoError(t, err)
	assert.Equal(t, "imported-site", template.Name)
	assert.Equal(t, "Imported Site", template.DisplayName)
	assert.Equal(t, "https://imported.com", template.BaseURL)
	assert.Equal(t, "api_key", template.AuthMethod)
	assert.Equal(t, `{"parser": "config"}`, template.ParserConfig)
	assert.Equal(t, "Imported description", template.Description)
	assert.Equal(t, "2.0.0", template.Version)
	assert.Equal(t, "Import Author", template.Author)
}

// TestSiteTemplateFromExportNilParserConfig 测试空解析器配置的导入
func TestSiteTemplateFromExportNilParserConfig(t *testing.T) {
	export := &SiteTemplateExport{
		Name:         "simple-site",
		DisplayName:  "Simple Site",
		BaseURL:      "https://simple.com",
		AuthMethod:   "cookie",
		ParserConfig: nil,
	}

	template := &SiteTemplate{}
	err := template.FromExport(export)

	assert.NoError(t, err)
	assert.Equal(t, "", template.ParserConfig)
}

// TestSiteTemplateExportFields 测试导出结构体字段
func TestSiteTemplateExportFields(t *testing.T) {
	export := SiteTemplateExport{
		Name:         "test",
		DisplayName:  "Test",
		BaseURL:      "https://test.com",
		AuthMethod:   "cookie",
		ParserConfig: json.RawMessage(`{}`),
		Description:  "desc",
		Version:      "1.0",
		Author:       "author",
	}

	// 测试 JSON 序列化
	data, err := json.Marshal(export)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"name":"test"`)
	assert.Contains(t, string(data), `"display_name":"Test"`)
	assert.Contains(t, string(data), `"base_url":"https://test.com"`)
	assert.Contains(t, string(data), `"auth_method":"cookie"`)
}
