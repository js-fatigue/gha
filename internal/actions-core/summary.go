package ac

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const summaryEnvVar = "GITHUB_STEP_SUMMARY"

// Summary builds a GitHub job step summary as HTML, flushed to GITHUB_STEP_SUMMARY.
// Use the package-level JobSummary instance, or create a fresh one with new(Summary).
type Summary struct {
	buf strings.Builder
}

// JobSummary is the package-level Summary instance for the current step.
var JobSummary = new(Summary)

// SummaryTableCell is one cell in an AddTable row.
// When Header is true the cell is rendered as <th>; otherwise as <td>.
// Colspan and Rowspan map to the HTML attributes of the same names.
type SummaryTableCell struct {
	Data    string
	Header  bool
	Colspan string
	Rowspan string
}

// SummaryImageOptions holds optional width and height for AddImage.
type SummaryImageOptions struct {
	Width  string
	Height string
}

// SummaryWriteOptions controls how Write flushes the buffer to disk.
type SummaryWriteOptions struct {
	Overwrite bool // if true the summary file is truncated before writing
}

// --- internal helpers ---

// htmlAttr is a key/value HTML attribute pair, kept as a slice to preserve order.
type htmlAttr struct{ key, val string }

// wrapHTML renders an HTML element. Pass a nil content pointer for void elements
// (e.g. <hr>, <br>, <img>) — no closing tag is emitted.
func wrapHTML(tag string, content *string, attrs ...htmlAttr) string {
	var sb strings.Builder
	sb.WriteByte('<')
	sb.WriteString(tag)
	for _, a := range attrs {
		fmt.Fprintf(&sb, ` %s="%s"`, a.key, a.val)
	}
	sb.WriteByte('>')
	if content != nil {
		sb.WriteString(*content)
		fmt.Fprintf(&sb, "</%s>", tag)
	}
	return sb.String()
}

func strContent(s string) *string { return &s }

// --- Summary methods ---

func (s *Summary) filePath() (string, error) {
	p := os.Getenv(summaryEnvVar)
	if p == "" {
		return "", fmt.Errorf(
			"unable to find environment variable for $%s; check if your runtime supports job summaries",
			summaryEnvVar,
		)
	}
	return p, nil
}

// Stringify returns the current buffer contents without flushing.
func (s *Summary) Stringify() string { return s.buf.String() }

// IsEmptyBuffer reports whether the buffer is empty.
func (s *Summary) IsEmptyBuffer() bool { return s.buf.Len() == 0 }

// EmptyBuffer clears the buffer and returns s for chaining.
func (s *Summary) EmptyBuffer() *Summary {
	s.buf.Reset()
	return s
}

// Write flushes the buffer to GITHUB_STEP_SUMMARY and empties the buffer.
// Content is appended by default; set Overwrite to truncate the file first.
func (s *Summary) Write(opts *SummaryWriteOptions) error {
	p, err := s.filePath()
	if err != nil {
		return err
	}
	flags := os.O_APPEND | os.O_WRONLY | os.O_CREATE
	if opts != nil && opts.Overwrite {
		flags = os.O_TRUNC | os.O_WRONLY | os.O_CREATE
	}
	f, err := os.OpenFile(p, flags, 0644)
	if err != nil {
		return fmt.Errorf("opening %s file: %w", summaryEnvVar, err)
	}
	defer f.Close()
	if _, err = fmt.Fprint(f, s.buf.String()); err != nil {
		return err
	}
	s.EmptyBuffer()
	return nil
}

// Clear empties the buffer and truncates the summary file.
func (s *Summary) Clear() error {
	return s.EmptyBuffer().Write(&SummaryWriteOptions{Overwrite: true})
}

// AddRaw appends text to the buffer. Pass true as the optional addEOL argument
// to append an OS line ending after the text (default: false).
func (s *Summary) AddRaw(text string, addEOL ...bool) *Summary {
	s.buf.WriteString(text)
	if len(addEOL) > 0 && addEOL[0] {
		return s.AddEOL()
	}
	return s
}

// AddEOL appends an OS-appropriate line ending to the buffer.
func (s *Summary) AddEOL() *Summary {
	eol := "\n"
	if runtime.GOOS == "windows" {
		eol = "\r\n"
	}
	s.buf.WriteString(eol)
	return s
}

// AddCodeBlock appends a syntax-highlighted <pre><code> block. lang is optional.
func (s *Summary) AddCodeBlock(code, lang string) *Summary {
	var attrs []htmlAttr
	if lang != "" {
		attrs = append(attrs, htmlAttr{"lang", lang})
	}
	element := wrapHTML("pre", strContent(wrapHTML("code", strContent(code))), attrs...)
	return s.AddRaw(element).AddEOL()
}

// AddList appends an HTML list. Set ordered true for <ol>, false for <ul>.
func (s *Summary) AddList(items []string, ordered bool) *Summary {
	tag := "ul"
	if ordered {
		tag = "ol"
	}
	var inner strings.Builder
	for _, item := range items {
		inner.WriteString(wrapHTML("li", strContent(item)))
	}
	return s.AddRaw(wrapHTML(tag, strContent(inner.String()))).AddEOL()
}

// AddTable appends an HTML table from a slice of rows.
// Cells with Header true are rendered as <th>; others as <td>.
// Colspan and Rowspan are included as HTML attributes when non-empty.
// Plain text cells can be created as SummaryTableCell{Data: "text"}.
func (s *Summary) AddTable(rows [][]SummaryTableCell) *Summary {
	var tableBody strings.Builder
	for _, row := range rows {
		var cells strings.Builder
		for _, cell := range row {
			tag := "td"
			if cell.Header {
				tag = "th"
			}
			var attrs []htmlAttr
			if cell.Colspan != "" {
				attrs = append(attrs, htmlAttr{"colspan", cell.Colspan})
			}
			if cell.Rowspan != "" {
				attrs = append(attrs, htmlAttr{"rowspan", cell.Rowspan})
			}
			cells.WriteString(wrapHTML(tag, strContent(cell.Data), attrs...))
		}
		tableBody.WriteString(wrapHTML("tr", strContent(cells.String())))
	}
	return s.AddRaw(wrapHTML("table", strContent(tableBody.String()))).AddEOL()
}

// AddDetails appends a <details> element with a <summary> label and body content.
func (s *Summary) AddDetails(label, content string) *Summary {
	inner := wrapHTML("summary", strContent(label)) + content
	return s.AddRaw(wrapHTML("details", strContent(inner))).AddEOL()
}

// AddImage appends an <img> element. opts is optional.
func (s *Summary) AddImage(src, alt string, opts *SummaryImageOptions) *Summary {
	attrs := []htmlAttr{{"src", src}, {"alt", alt}}
	if opts != nil {
		if opts.Width != "" {
			attrs = append(attrs, htmlAttr{"width", opts.Width})
		}
		if opts.Height != "" {
			attrs = append(attrs, htmlAttr{"height", opts.Height})
		}
	}
	return s.AddRaw(wrapHTML("img", nil, attrs...)).AddEOL()
}

// AddHeading appends an HTML heading element. level must be 1–6; any other value defaults to 1.
func (s *Summary) AddHeading(text string, level int) *Summary {
	tag := fmt.Sprintf("h%d", level)
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
	default:
		tag = "h1"
	}
	return s.AddRaw(wrapHTML(tag, strContent(text))).AddEOL()
}

// AddSeparator appends an <hr> element.
func (s *Summary) AddSeparator() *Summary {
	return s.AddRaw(wrapHTML("hr", nil)).AddEOL()
}

// AddBreak appends a <br> element.
func (s *Summary) AddBreak() *Summary {
	return s.AddRaw(wrapHTML("br", nil)).AddEOL()
}

// AddQuote appends a <blockquote> element. cite is optional.
func (s *Summary) AddQuote(text, cite string) *Summary {
	var attrs []htmlAttr
	if cite != "" {
		attrs = append(attrs, htmlAttr{"cite", cite})
	}
	return s.AddRaw(wrapHTML("blockquote", strContent(text), attrs...)).AddEOL()
}

// AddLink appends an <a> element.
func (s *Summary) AddLink(text, href string) *Summary {
	return s.AddRaw(wrapHTML("a", strContent(text), htmlAttr{"href", href})).AddEOL()
}
