package document

import (
	"strings"

	"go.uber.org/zap"
)

// DocxTextExtractor handles text extraction and manipulation for DOCX files
type DocxTextExtractor struct {
	logger    *zap.Logger
	mergeRuns bool
}

// NewDocxTextExtractor creates a new text extractor
func NewDocxTextExtractor(logger *zap.Logger) *DocxTextExtractor {
	return &DocxTextExtractor{
		logger:    logger,
		mergeRuns: true, // Default to merging runs for better translation
	}
}

// ExtractParagraphText extracts all text from a paragraph
func (e *DocxTextExtractor) ExtractParagraphText(para *Paragraph) string {
	if para == nil {
		return ""
	}

	var texts []string
	for _, run := range para.Runs {
		if text := e.extractRunText(&run); text != "" {
			texts = append(texts, text)
		}
	}

	return strings.Join(texts, "")
}

// extractRunText extracts text from a run
func (e *DocxTextExtractor) extractRunText(run *Run) string {
	if run == nil {
		return ""
	}

	// Handle text element
	if run.Text != nil {
		return run.Text.Text
	}

	// Handle tab
	if run.Tab != nil {
		return "\t"
	}

	// Handle break
	if run.Break != nil {
		if run.Break.Type == "page" {
			return "\n\n" // Page break as double newline
		}
		return "\n"
	}

	// Skip drawings and other complex elements for now
	return ""
}

// UpdateParagraphText updates paragraph with translated text
func (e *DocxTextExtractor) UpdateParagraphText(para *Paragraph, translatedText string) {
	if para == nil || translatedText == "" {
		return
	}

	// If merge runs is enabled, create a single run with all text
	if e.mergeRuns {
		e.updateWithSingleRun(para, translatedText)
		return
	}

	// Otherwise, try to preserve run structure
	e.updatePreservingRuns(para, translatedText)
}

// updateWithSingleRun replaces all runs with a single run containing translated text
func (e *DocxTextExtractor) updateWithSingleRun(para *Paragraph, translatedText string) {
	// Preserve first run's properties if available
	var props *RunProps
	if len(para.Runs) > 0 && para.Runs[0].Properties != nil {
		props = para.Runs[0].Properties
	}

	// Create new run with translated text
	newRun := Run{
		Properties: props,
		Text: &Text{
			Text:  translatedText,
			Space: "preserve",
		},
	}

	// Replace all runs
	para.Runs = []Run{newRun}
}

// updatePreservingRuns tries to preserve run structure while updating text
func (e *DocxTextExtractor) updatePreservingRuns(para *Paragraph, translatedText string) {
	// Collect run information
	type runInfo struct {
		index      int
		length     int
		properties *RunProps
		isText     bool
	}

	var runs []runInfo
	totalLength := 0

	for i, run := range para.Runs {
		text := e.extractRunText(&run)
		if text != "" {
			runs = append(runs, runInfo{
				index:      i,
				length:     len(text),
				properties: run.Properties,
				isText:     run.Text != nil,
			})
			totalLength += len(text)
		}
	}

	if len(runs) == 0 {
		return
	}

	// Distribute translated text proportionally
	translatedRunes := []rune(translatedText)
	position := 0

	for _, info := range runs {
		if !info.isText {
			continue // Skip non-text runs
		}

		// Calculate proportional length
		proportion := float64(info.length) / float64(totalLength)
		newLength := int(float64(len(translatedRunes)) * proportion)

		// Handle last run
		if info.index == runs[len(runs)-1].index {
			newLength = len(translatedRunes) - position
		}

		// Extract text portion
		if position+newLength > len(translatedRunes) {
			newLength = len(translatedRunes) - position
		}

		if newLength > 0 && position < len(translatedRunes) {
			newText := string(translatedRunes[position : position+newLength])
			para.Runs[info.index].Text = &Text{
				Text:  newText,
				Space: "preserve",
			}
			position += newLength
		}
	}
}

// MergeAdjacentRuns merges runs with identical formatting
func (e *DocxTextExtractor) MergeAdjacentRuns(para *Paragraph) []MergedRun {
	if para == nil || len(para.Runs) == 0 {
		return nil
	}

	var merged []MergedRun
	var currentMerge *MergedRun

	for i, run := range para.Runs {
		text := e.extractRunText(&run)
		if text == "" && run.Drawing == nil {
			continue // Skip empty runs without drawings
		}

		// Check if we can merge with current
		if currentMerge != nil && e.canMergeRuns(&para.Runs[currentMerge.EndIndex], &run) {
			currentMerge.Text += text
			currentMerge.EndIndex = i
		} else {
			// Start new merge
			if currentMerge != nil {
				merged = append(merged, *currentMerge)
			}
			currentMerge = &MergedRun{
				Text:       text,
				Properties: run.Properties,
				StartIndex: i,
				EndIndex:   i,
			}
		}
	}

	// Add final merge
	if currentMerge != nil {
		merged = append(merged, *currentMerge)
	}

	return merged
}

// canMergeRuns checks if two runs can be merged
func (e *DocxTextExtractor) canMergeRuns(run1, run2 *Run) bool {
	// Don't merge if either has special elements
	if run1.Drawing != nil || run2.Drawing != nil {
		return false
	}
	if run1.Tab != nil || run2.Tab != nil {
		return false
	}
	if run1.Break != nil || run2.Break != nil {
		return false
	}

	// Compare properties
	return e.compareRunProperties(run1.Properties, run2.Properties)
}

// compareRunProperties compares two run properties for equality
func (e *DocxTextExtractor) compareRunProperties(p1, p2 *RunProps) bool {
	// If both nil, they're equal
	if p1 == nil && p2 == nil {
		return true
	}

	// If one is nil, they're not equal
	if p1 == nil || p2 == nil {
		return false
	}

	// Compare key formatting properties
	if !e.compareBold(p1.Bold, p2.Bold) {
		return false
	}
	if !e.compareItalic(p1.Italic, p2.Italic) {
		return false
	}
	if !e.compareUnderline(p1.Underline, p2.Underline) {
		return false
	}
	if !e.compareColor(p1.Color, p2.Color) {
		return false
	}
	if !e.compareFontSize(p1.Size, p2.Size) {
		return false
	}

	return true
}

// compareBold compares two bold properties
func (e *DocxTextExtractor) compareBold(b1, b2 *Bold) bool {
	if b1 == nil && b2 == nil {
		return true
	}
	if b1 == nil || b2 == nil {
		return false
	}
	return b1.Val == b2.Val
}

// compareItalic compares two italic properties
func (e *DocxTextExtractor) compareItalic(i1, i2 *Italic) bool {
	if i1 == nil && i2 == nil {
		return true
	}
	if i1 == nil || i2 == nil {
		return false
	}
	return i1.Val == i2.Val
}

// compareUnderline compares underline properties
func (e *DocxTextExtractor) compareUnderline(u1, u2 *Underline) bool {
	if u1 == nil && u2 == nil {
		return true
	}
	if u1 == nil || u2 == nil {
		return false
	}
	return u1.Val == u2.Val
}

// compareColor compares color properties
func (e *DocxTextExtractor) compareColor(c1, c2 *Color) bool {
	if c1 == nil && c2 == nil {
		return true
	}
	if c1 == nil || c2 == nil {
		return false
	}
	return c1.Val == c2.Val
}

// compareFontSize compares font size properties
func (e *DocxTextExtractor) compareFontSize(s1, s2 *FontSize) bool {
	if s1 == nil && s2 == nil {
		return true
	}
	if s1 == nil || s2 == nil {
		return false
	}
	return s1.Val == s2.Val
}

// SplitTranslatedText splits translated text back to original run structure
func (e *DocxTextExtractor) SplitTranslatedText(translatedText string, mergedRuns []MergedRun) []string {
	if len(mergedRuns) == 0 {
		return nil
	}

	// Calculate total original length
	totalLength := 0
	for _, mr := range mergedRuns {
		totalLength += len(mr.Text)
	}

	if totalLength == 0 {
		return nil
	}

	// Split proportionally
	result := make([]string, len(mergedRuns))
	translatedRunes := []rune(translatedText)
	position := 0

	for i, mr := range mergedRuns {
		// Calculate proportional length
		proportion := float64(len(mr.Text)) / float64(totalLength)
		newLength := int(float64(len(translatedRunes)) * proportion)

		// Handle last segment
		if i == len(mergedRuns)-1 {
			newLength = len(translatedRunes) - position
		}

		// Extract segment
		if position+newLength > len(translatedRunes) {
			newLength = len(translatedRunes) - position
		}

		if newLength > 0 && position < len(translatedRunes) {
			result[i] = string(translatedRunes[position : position+newLength])
			position += newLength
		} else {
			result[i] = ""
		}
	}

	return result
}

// MergedRun represents runs that have been merged
type MergedRun struct {
	Text       string
	Properties *RunProps
	StartIndex int
	EndIndex   int
}