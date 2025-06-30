package document

import (
	"context"
	"encoding/xml"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestDocxTextExtractor(t *testing.T) {
	logger := zap.NewNop()
	extractor := NewDocxTextExtractor(logger)

	t.Run("ExtractParagraphText", func(t *testing.T) {
		para := &Paragraph{
			Runs: []Run{
				{
					Text: &Text{Text: "Hello "},
				},
				{
					Text: &Text{Text: "world"},
				},
				{
					Text: &Text{Text: "!"},
				},
			},
		}

		text := extractor.ExtractParagraphText(para)
		expected := "Hello world!"
		if text != expected {
			t.Errorf("Expected %q, got %q", expected, text)
		}
	})

	t.Run("ExtractWithSpecialElements", func(t *testing.T) {
		para := &Paragraph{
			Runs: []Run{
				{
					Text: &Text{Text: "Line 1"},
				},
				{
					Tab: &Tab{},
				},
				{
					Text: &Text{Text: "Line 2"},
				},
				{
					Break: &Break{},
				},
				{
					Text: &Text{Text: "Line 3"},
				},
			},
		}

		text := extractor.ExtractParagraphText(para)
		expected := "Line 1\tLine 2\nLine 3"
		if text != expected {
			t.Errorf("Expected %q, got %q", expected, text)
		}
	})
}

func TestDocxProtector(t *testing.T) {
	protector := NewDocxProtector()

	t.Run("ProtectFieldCodes", func(t *testing.T) {
		text := "Page {PAGE} of {NUMPAGES}"
		protected := protector.Protect(text)

		// Should have placeholders
		if !strings.Contains(protected, "@@DOCX_FIELD_") {
			t.Errorf("Expected field codes to be protected, got: %s", protected)
		}

		// Restore should bring back original
		restored := protector.Restore(protected)
		if restored != text {
			t.Errorf("Expected %q, got %q", text, restored)
		}
	})

	t.Run("ProtectMultiplePatterns", func(t *testing.T) {
		text := "See {REF bookmark1} on page [PAGE]"
		protected := protector.Protect(text)

		// Should contain placeholders for both patterns
		if !strings.Contains(protected, "@@DOCX_FIELD_") {
			t.Error("Expected field code to be protected")
		}
		if !strings.Contains(protected, "@@DOCX_PAGE_") {
			t.Error("Expected page marker to be protected")
		}

		// Restore
		restored := protector.Restore(protected)
		if restored != text {
			t.Errorf("Expected %q, got %q", text, restored)
		}
	})
}

func TestDocxStructuresXML(t *testing.T) {
	t.Run("MarshalParagraph", func(t *testing.T) {
		para := &Paragraph{
			Runs: []Run{
				{
					Properties: &RunProps{
						Bold: &Bold{},
					},
					Text: &Text{
						Text: "Bold text",
					},
				},
			},
		}

		data, err := xml.Marshal(para)
		if err != nil {
			t.Fatalf("Failed to marshal paragraph: %v", err)
		}

		// Check XML contains expected elements
		xmlStr := string(data)
		if !strings.Contains(xmlStr, "<p>") {
			t.Error("Expected <p> element in XML")
		}
		if !strings.Contains(xmlStr, "<r>") {
			t.Error("Expected <r> element in XML")
		}
		if !strings.Contains(xmlStr, "<t>Bold text</t>") {
			t.Error("Expected text content in XML")
		}
	})

	t.Run("UnmarshalParagraph", func(t *testing.T) {
		xmlData := `<p>
			<r>
				<rPr><b/></rPr>
				<t>Test text</t>
			</r>
		</p>`

		var para Paragraph
		err := xml.Unmarshal([]byte(xmlData), &para)
		if err != nil {
			t.Fatalf("Failed to unmarshal paragraph: %v", err)
		}

		if len(para.Runs) != 1 {
			t.Errorf("Expected 1 run, got %d", len(para.Runs))
		}

		if para.Runs[0].Text == nil || para.Runs[0].Text.Text != "Test text" {
			t.Error("Failed to extract text from paragraph")
		}
	})
}

func TestDocxProcessor(t *testing.T) {
	logger := zap.NewNop()
	opts := ProcessorOptions{
		ChunkSize:    1000,
		ChunkOverlap: 100,
	}

	processor, err := NewDocxProcessor(opts, logger)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	t.Run("GetFormat", func(t *testing.T) {
		format := processor.GetFormat()
		if format != FormatDOCX {
			t.Errorf("Expected format %s, got %s", FormatDOCX, format)
		}
	})

	t.Run("ProtectContent", func(t *testing.T) {
		text := "Page {PAGE} of {NUMPAGES}"
		protected := processor.ProtectContent(text, nil)

		if !strings.Contains(protected, "@@DOCX_FIELD_") {
			t.Error("Expected content to be protected")
		}
	})
}

// Simple mock translator for testing
func mockTranslator(ctx context.Context, text string) (string, error) {
	// Simple mock: prepend "Translated: " to the text
	return "Translated: " + text, nil
}

func TestDocxProcessing(t *testing.T) {
	logger := zap.NewNop()
	processor, _ := NewDocxProcessor(ProcessorOptions{}, logger)

	t.Run("ConvertToDocument", func(t *testing.T) {
		wordDoc := &WordDocument{
			Body: Body{
				Paragraphs: []Paragraph{
					{
						Runs: []Run{
							{
								Text: &Text{Text: "Hello world"},
							},
						},
					},
					{
						Runs: []Run{
							{
								Text: &Text{Text: "Second paragraph"},
							},
						},
					},
				},
			},
		}

		doc := processor.convertToDocument(wordDoc)

		if len(doc.Blocks) != 2 {
			t.Errorf("Expected 2 blocks, got %d", len(doc.Blocks))
		}

		// Check protection was applied
		block1 := doc.Blocks[0].GetContent()
		if !strings.Contains(block1, "Hello world") && !strings.Contains(block1, "@@DOCX_") {
			t.Error("Expected first block to contain text or protection markers")
		}
	})
}

func TestMergeAdjacentRuns(t *testing.T) {
	logger := zap.NewNop()
	extractor := NewDocxTextExtractor(logger)

	t.Run("MergeSameFormatting", func(t *testing.T) {
		para := &Paragraph{
			Runs: []Run{
				{
					Properties: &RunProps{Bold: &Bold{}},
					Text:       &Text{Text: "Hello "},
				},
				{
					Properties: &RunProps{Bold: &Bold{}},
					Text:       &Text{Text: "world"},
				},
				{
					Properties: &RunProps{}, // Different properties
					Text:       &Text{Text: "!"},
				},
			},
		}

		merged := extractor.MergeAdjacentRuns(para)

		if len(merged) != 2 {
			t.Errorf("Expected 2 merged runs, got %d", len(merged))
		}

		if merged[0].Text != "Hello world" {
			t.Errorf("Expected first merged text to be 'Hello world', got %q", merged[0].Text)
		}

		if merged[1].Text != "!" {
			t.Errorf("Expected second merged text to be '!', got %q", merged[1].Text)
		}
	})
}

func TestSplitTranslatedText(t *testing.T) {
	logger := zap.NewNop()
	extractor := NewDocxTextExtractor(logger)

	t.Run("ProportionalSplit", func(t *testing.T) {
		mergedRuns := []MergedRun{
			{Text: "Hello ", StartIndex: 0, EndIndex: 0},
			{Text: "world!", StartIndex: 1, EndIndex: 1},
		}

		translated := "你好 世界！"
		split := extractor.SplitTranslatedText(translated, mergedRuns)

		if len(split) != 2 {
			t.Errorf("Expected 2 splits, got %d", len(split))
		}

		// The split should be proportional to original lengths
		// Original: "Hello " (6 chars) + "world!" (6 chars) = 12 chars
		// Translated: "你好 世界！" (6 chars)
		// Each part should get ~50% of the translation
		if split[0] == "" || split[1] == "" {
			t.Error("Expected non-empty splits")
		}
	})
}