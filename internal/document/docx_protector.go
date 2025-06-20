package document

import (
	"fmt"
	"regexp"
	"strings"
)

// DocxProtector handles content protection for DOCX documents
type DocxProtector struct {
	placeholders map[string]string
	counter      int
	patterns     []ProtectionPattern
}

// ProtectionPattern defines a pattern to protect
type ProtectionPattern struct {
	Name    string
	Pattern *regexp.Regexp
	Replace func(match string) string
}

// NewDocxProtector creates a new DOCX protector
func NewDocxProtector() *DocxProtector {
	p := &DocxProtector{
		placeholders: make(map[string]string),
		counter:      0,
	}

	// Define protection patterns
	p.patterns = []ProtectionPattern{
		{
			Name:    "field_code",
			Pattern: regexp.MustCompile(`\{[^}]+\}`),
			Replace: p.protectFieldCode,
		},
		{
			Name:    "bookmark",
			Pattern: regexp.MustCompile(`\[BOOKMARK:[^]]+\]`),
			Replace: p.protectBookmark,
		},
		{
			Name:    "hyperlink",
			Pattern: regexp.MustCompile(`\[HYPERLINK:[^]]+\]`),
			Replace: p.protectHyperlink,
		},
		{
			Name:    "page_number",
			Pattern: regexp.MustCompile(`\[PAGE\]|\[NUMPAGES\]`),
			Replace: p.protectPageNumber,
		},
	}

	return p
}

// Protect applies all protection patterns to the text
func (p *DocxProtector) Protect(text string) string {
	protected := text

	// Apply each protection pattern
	for _, pattern := range p.patterns {
		protected = pattern.Pattern.ReplaceAllStringFunc(protected, pattern.Replace)
	}

	// Protect XML-like tags that might appear in extracted text
	protected = p.protectXMLTags(protected)

	return protected
}

// Restore replaces all placeholders with original content
func (p *DocxProtector) Restore(text string) string {
	restored := text

	// Restore in reverse order to handle nested protections
	for placeholder, original := range p.placeholders {
		restored = strings.ReplaceAll(restored, placeholder, original)
	}

	return restored
}

// protectFieldCode protects field codes
func (p *DocxProtector) protectFieldCode(match string) string {
	p.counter++
	placeholder := fmt.Sprintf("@@DOCX_FIELD_%d@@", p.counter)
	p.placeholders[placeholder] = match
	return placeholder
}

// protectBookmark protects bookmarks
func (p *DocxProtector) protectBookmark(match string) string {
	p.counter++
	placeholder := fmt.Sprintf("@@DOCX_BOOKMARK_%d@@", p.counter)
	p.placeholders[placeholder] = match
	return placeholder
}

// protectHyperlink protects hyperlinks
func (p *DocxProtector) protectHyperlink(match string) string {
	p.counter++
	placeholder := fmt.Sprintf("@@DOCX_LINK_%d@@", p.counter)
	p.placeholders[placeholder] = match
	return placeholder
}

// protectPageNumber protects page numbers
func (p *DocxProtector) protectPageNumber(match string) string {
	p.counter++
	placeholder := fmt.Sprintf("@@DOCX_PAGE_%d@@", p.counter)
	p.placeholders[placeholder] = match
	return placeholder
}

// protectXMLTags protects any XML-like tags that might appear
func (p *DocxProtector) protectXMLTags(text string) string {
	// Only protect tags that look like Word XML elements
	xmlPattern := regexp.MustCompile(`</?w:[^>]+>`)
	return xmlPattern.ReplaceAllStringFunc(text, func(match string) string {
		p.counter++
		placeholder := fmt.Sprintf("@@DOCX_XML_%d@@", p.counter)
		p.placeholders[placeholder] = match
		return placeholder
	})
}

// ProtectRun protects special content within a run
func (p *DocxProtector) ProtectRun(run *Run) {
	if run == nil || run.Text == nil {
		return
	}

	// Protect the text content
	run.Text.Text = p.Protect(run.Text.Text)
}

// RestoreRun restores protected content in a run
func (p *DocxProtector) RestoreRun(run *Run) {
	if run == nil || run.Text == nil {
		return
	}

	// Restore the text content
	run.Text.Text = p.Restore(run.Text.Text)
}

// GetPlaceholderCount returns the number of placeholders created
func (p *DocxProtector) GetPlaceholderCount() int {
	return len(p.placeholders)
}

// Reset clears all placeholders
func (p *DocxProtector) Reset() {
	p.placeholders = make(map[string]string)
	p.counter = 0
}