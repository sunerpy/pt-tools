package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

// DumpTableJSON snapshots an entire DB table to a JSON file under outDir.
// Filename format: {tableName}_v{schemaFromVer}_to_v{schemaToVer}_{ts}.json
// outDir is created if it does not exist.
// Returns the absolute path of the written file.
// Reads via SELECT * with []map[string]any to remain decoupled from concrete model structs.
func DumpTableJSON(db *gorm.DB, tableName, outDir string, schemaFromVer, schemaToVer int) (string, error) {
	if outDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		outDir = filepath.Join(homeDir, ".pt-tools", "backups")
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	rows, err := db.Table(tableName).Rows()
	if err != nil {
		return "", fmt.Errorf("failed to query table %s: %w", tableName, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("failed to get columns for table %s: %w", tableName, err)
	}

	dataRows := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if scanErr := rows.Scan(valuePtrs...); scanErr != nil {
			return "", fmt.Errorf("failed to scan row from table %s: %w", tableName, scanErr)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		dataRows = append(dataRows, row)
	}

	if rows.Err() != nil {
		return "", fmt.Errorf("error iterating rows from table %s: %w", tableName, rows.Err())
	}

	jsonData, err := json.Marshal(dataRows)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	filename := fmt.Sprintf("%s_v%d_to_v%d_%s.json", tableName, schemaFromVer, schemaToVer, ts)
	filePath := filepath.Join(outDir, filename)

	if writeErr := os.WriteFile(filePath, jsonData, 0o644); writeErr != nil {
		return "", fmt.Errorf("failed to write backup file %s: %w", filePath, writeErr)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	return absPath, nil
}
