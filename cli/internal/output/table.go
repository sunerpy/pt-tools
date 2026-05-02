package output

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

var (
	green  = color.New(color.FgGreen)
	red    = color.New(color.FgRed)
	yellow = color.New(color.FgYellow)
	cyan   = color.New(color.FgCyan)
	bold   = color.New(color.Bold)
)

// Success prints a success message.
func Success(format string, args ...any) {
	green.Printf(format+"\n", args...)
}

// Error prints an error message.
func Error(format string, args ...any) {
	red.Printf(format+"\n", args...)
}

// Warn prints a warning message.
func Warn(format string, args ...any) {
	yellow.Printf(format+"\n", args...)
}

// Info prints an informational message.
func Info(format string, args ...any) {
	cyan.Printf(format+"\n", args...)
}

// PrintTable renders a table to stdout.
func PrintTable(headers []string, rows [][]any) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleRounded)

	headerRow := table.Row{}
	for _, h := range headers {
		headerRow = append(headerRow, h)
	}
	t.AppendHeader(headerRow)

	for _, row := range rows {
		t.AppendRow(table.Row(row))
	}

	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, Align: text.AlignRight},
	})
	t.Render()
}

// FreeStatus returns a colored string for torrent discount level.
func FreeStatus(level string) string {
	switch level {
	case "free", "Free", "FREE":
		return green.Sprint("Free")
	case "2x", "2XFree", "2X Free":
		return green.Sprint("2x")
	case "50%", "Half", "50% Free":
		return yellow.Sprint("50%")
	case "30%", "30% Free":
		return yellow.Sprint("30%")
	case "normal", "Normal", "":
		return red.Sprint("Normal")
	default:
		return level
	}
}

// TagString returns a formatted tag string.
func TagString(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	result := ""
	for i, t := range tags {
		if i > 0 {
			result += " "
		}
		result += "[" + t + "]"
	}
	return result
}

// Spinner creates a simple text spinner message.
func Spinner(msg string) {
	fmt.Printf("\r  %s...", msg)
}

// Done replaces the spinner with a checkmark.
func Done() {
	fmt.Printf("\r  ✓ Done\n")
}
