package nfo

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// xmlWriter hand-written XML encoder.
// NOT using encoding/xml.Marshal because:
//  1. Does not guarantee child tag order (Kodi NFO spec requires specific order)
//  2. Does not support selective self-closing for empty elements
//  3. Does not support comments
type xmlWriter struct {
	w      *bufio.Writer
	indent string
	depth  int
}

// newXMLWriter creates encoder and writes XML declaration header.
// encoding is fixed to UTF-8 (Jellyfin/Kodi requirement), not parameterized.
func newXMLWriter(w io.Writer) *xmlWriter {
	bw := bufio.NewWriter(w)
	x := &xmlWriter{w: bw, indent: "  "} // 2-space indentation
	// XML declaration header
	_, _ = bw.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n")
	return x
}

// writeElement writes <name>value</name> single-line element.
// Empty value writes <name/> self-closing.
// Correctly escapes XML special chars (< > & " ').
func (x *xmlWriter) writeElement(name, value string) error {
	if err := x.writeIndent(); err != nil {
		return err
	}
	if value == "" {
		_, err := fmt.Fprintf(x.w, "<%s/>\n", name)
		return err
	}
	escaped := escape(value)
	_, err := fmt.Fprintf(x.w, "<%s>%s</%s>\n", name, escaped, name)
	return err
}

// writeElementAttr writes <name attr1="v1" attr2="v2">value</name>.
// attrs is even-length key/value list (simplified API, avoids map ordering).
// Empty value writes <name attr1="v1"/> self-closing.
func (x *xmlWriter) writeElementAttr(name string, attrs []string, value string) error {
	if err := x.writeIndent(); err != nil {
		return err
	}

	// Build attributes string
	var attrStr strings.Builder
	for i := 0; i < len(attrs); i += 2 {
		if i > 0 {
			attrStr.WriteString(" ")
		}
		attrStr.WriteString(attrs[i])
		attrStr.WriteString("=\"")
		attrStr.WriteString(escape(attrs[i+1]))
		attrStr.WriteString("\"")
	}

	if value == "" {
		var out strings.Builder
		out.WriteString("<")
		out.WriteString(name)
		if attrStr.Len() > 0 {
			out.WriteString(" ")
			out.WriteString(attrStr.String())
		}
		out.WriteString("/>\n")
		_, err := x.w.WriteString(out.String())
		return err
	}

	escaped := escape(value)
	var out strings.Builder
	out.WriteString("<")
	out.WriteString(name)
	if attrStr.Len() > 0 {
		out.WriteString(" ")
		out.WriteString(attrStr.String())
	}
	out.WriteString(">")
	out.WriteString(escaped)
	out.WriteString("</")
	out.WriteString(name)
	out.WriteString(">\n")

	_, err := x.w.WriteString(out.String())
	return err
}

// writeElementInt convenience method: integer value.
// (0 value skip is caller's responsibility).
func (x *xmlWriter) writeElementInt(name string, value int) error {
	return x.writeElement(name, fmt.Sprintf("%d", value))
}

// writeElementFloat convenience method: floating point with 1 decimal (%.1f).
func (x *xmlWriter) writeElementFloat(name string, value float64) error {
	return x.writeElement(name, fmt.Sprintf("%.1f", value))
}

// openBlock writes <name>\n and increases indentation.
// Does NOT escape name (assumes caller passes valid XML tag name).
func (x *xmlWriter) openBlock(name string) error {
	if err := x.writeIndent(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(x.w, "<%s>\n", name)
	if err == nil {
		x.depth++
	}
	return err
}

// openBlockAttr writes <name attr1="v1" ...>\n and increases indentation.
//
//nolint:unused // reserved for Task 10 (Kodi NFO writer with attribute support)
func (x *xmlWriter) openBlockAttr(name string, attrs []string) error {
	if err := x.writeIndent(); err != nil {
		return err
	}

	// Build attributes string
	var attrStr strings.Builder
	for i := 0; i < len(attrs); i += 2 {
		if i > 0 {
			attrStr.WriteString(" ")
		}
		attrStr.WriteString(attrs[i])
		attrStr.WriteString("=\"")
		attrStr.WriteString(escape(attrs[i+1]))
		attrStr.WriteString("\"")
	}

	var out strings.Builder
	out.WriteString("<")
	out.WriteString(name)
	if attrStr.Len() > 0 {
		out.WriteString(" ")
		out.WriteString(attrStr.String())
	}
	out.WriteString(">\n")

	_, err := x.w.WriteString(out.String())
	if err == nil {
		x.depth++
	}
	return err
}

// closeBlock decreases indentation and writes </name>\n.
func (x *xmlWriter) closeBlock(name string) error {
	if x.depth > 0 {
		x.depth--
	}
	if err := x.writeIndent(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(x.w, "</%s>\n", name)
	return err
}

// writeBlock convenience method: openBlock + inner callback + closeBlock.
// Returns inner error if callback fails.
func (x *xmlWriter) writeBlock(name string, inner func() error) error {
	if err := x.openBlock(name); err != nil {
		return err
	}
	if err := inner(); err != nil {
		return err
	}
	return x.closeBlock(name)
}

// writeComment writes <!-- ... --> comment.
// Replaces -- with - - (XML spec forbids --).
func (x *xmlWriter) writeComment(text string) error {
	if err := x.writeIndent(); err != nil {
		return err
	}
	safe := strings.ReplaceAll(text, "--", "- -")
	_, err := fmt.Fprintf(x.w, "<!-- %s -->\n", safe)
	return err
}

// writeRaw writes raw unescaped string (for JSON payload / pre-formatted block).
// Caller responsible for well-formed XML.
func (x *xmlWriter) writeRaw(s string) error {
	_, err := x.w.WriteString(s)
	return err
}

// writeIndent writes spaces based on current depth.
func (x *xmlWriter) writeIndent() error {
	for i := 0; i < x.depth; i++ {
		if _, err := x.w.WriteString(x.indent); err != nil {
			return err
		}
	}
	return nil
}

// flush flushes buffer to underlying writer. Must be called after all writes complete.
//
//nolint:unused // reserved for future use (explicit flush before file close)
func (x *xmlWriter) flush() error {
	return x.w.Flush()
}

// escape returns XML-escaped string (& < > " ' → entities).
// Uses encoding/xml.EscapeText to ensure consistency with standard.
func escape(s string) string {
	var buf strings.Builder
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
