package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContractTools_HasTenTools 验证 Phase 4 计划暴露的 MCP 工具数量为 10。
// 该集合需与 docs/design/phase4-mcp.md 第 5 节 "Tool Inventory" 保持同步。
func TestContractTools_HasTenTools(t *testing.T) {
	require.Len(t, ContractTools, 10, "Phase 4 应当暴露恰好 10 个 MCP 工具")
}

// TestContractTools_NamesUnique 验证工具名唯一，避免 MCP server 注册时冲突。
func TestContractTools_NamesUnique(t *testing.T) {
	seen := make(map[string]bool, len(ContractTools))
	for _, tool := range ContractTools {
		require.NotEmpty(t, tool.Name, "工具 Name 不可为空")
		assert.Falsef(t, seen[tool.Name], "工具名重复: %s", tool.Name)
		seen[tool.Name] = true
	}
	assert.Len(t, seen, len(ContractTools))
}

// TestContractTools_AllHaveInputSchema 验证每个工具都提供 InputSchema，
// 并且 schema 至少声明了 "type" 字段（JSON Schema 必填）。
func TestContractTools_AllHaveInputSchema(t *testing.T) {
	for _, tool := range ContractTools {
		require.NotNilf(t, tool.InputSchema, "工具 %s 缺少 InputSchema", tool.Name)
		assert.NotEmptyf(t, tool.InputSchema, "工具 %s InputSchema 为空", tool.Name)
		assert.Containsf(t, tool.InputSchema, "type", "工具 %s InputSchema 缺少 type 字段", tool.Name)
		assert.NotEmptyf(t, tool.Description, "工具 %s 缺少 Description", tool.Name)
	}
}
