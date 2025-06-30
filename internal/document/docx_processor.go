package document

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// DocxProcessor processes DOCX format documents
type DocxProcessor struct {
	opts      ProcessorOptions
	logger    *zap.Logger
	tempDir   string
	extractor *DocxTextExtractor
	protector *DocxProtector
}

// NewDocxProcessor creates a new DOCX processor
func NewDocxProcessor(opts ProcessorOptions, logger *zap.Logger) (*DocxProcessor, error) {
	// Set defaults
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = 2000
	}
	if opts.ChunkOverlap < 0 {
		opts.ChunkOverlap = 100
	}

	return &DocxProcessor{
		opts:      opts,
		logger:    logger,
		extractor: NewDocxTextExtractor(logger),
		protector: NewDocxProtector(),
	}, nil
}

// Parse parses a DOCX file into a Document
func (p *DocxProcessor) Parse(ctx context.Context, input io.Reader) (*Document, error) {
	// Read all data
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	// Create temporary directory for extraction
	tempDir, err := os.MkdirTemp("", "docx-parse-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	p.tempDir = tempDir
	defer p.cleanup()

	// Extract DOCX contents
	if err := p.extractDocx(data); err != nil {
		return nil, fmt.Errorf("failed to extract DOCX: %w", err)
	}

	// Parse document.xml
	wordDoc, err := p.parseDocumentXML()
	if err != nil {
		return nil, fmt.Errorf("failed to parse document.xml: %w", err)
	}

	// Convert to Document
	doc := p.convertToDocument(wordDoc)

	// Store DOCX data for later rendering
	doc.Metadata.CustomFields["docxData"] = data
	doc.Metadata.CustomFields["docxPath"] = tempDir

	return doc, nil
}

// Process processes the document through translation
func (p *DocxProcessor) Process(ctx context.Context, doc *Document, translator TranslateFunc) (*Document, error) {
	// The document already has protected content from Parse
	// Simply translate each block
	for i, block := range doc.Blocks {
		if !block.IsTranslatable() {
			continue
		}

		// Translate the block content (which already has protection applied)
		translatedText, err := translator(ctx, block.GetContent())
		if err != nil {
			p.logger.Warn("failed to translate block",
				zap.Int("index", i),
				zap.Error(err))
			continue
		}

		// Update the block with translated content
		block.SetContent(translatedText)
	}

	return doc, nil
}

// Render renders the document back to DOCX format
func (p *DocxProcessor) Render(ctx context.Context, doc *Document, output io.Writer) error {
	// Get original DOCX data
	docxData, ok := doc.Metadata.CustomFields["docxData"].([]byte)
	if !ok {
		return fmt.Errorf("original DOCX data not found in document metadata")
	}

	// Create new temp directory for rendering
	tempDir, err := os.MkdirTemp("", "docx-render-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract original DOCX
	reader := bytes.NewReader(docxData)
	zipReader, err := zip.NewReader(reader, int64(len(docxData)))
	if err != nil {
		return fmt.Errorf("failed to read DOCX: %w", err)
	}

	// Extract all files
	for _, file := range zipReader.File {
		if err := p.extractFile(file, tempDir); err != nil {
			return fmt.Errorf("failed to extract file %s: %w", file.Name, err)
		}
	}

	// Parse document.xml
	docPath := filepath.Join(tempDir, "word", "document.xml")
	wordDoc, err := p.parseXMLFile(docPath)
	if err != nil {
		return fmt.Errorf("failed to parse document.xml: %w", err)
	}

	// Update with translated content
	if err := p.updateDocumentContent(wordDoc, doc); err != nil {
		return fmt.Errorf("failed to update document content: %w", err)
	}

	// Write updated document.xml
	if err := p.writeXMLFile(docPath, wordDoc); err != nil {
		return fmt.Errorf("failed to write document.xml: %w", err)
	}

	// Create new DOCX
	if err := p.createDocx(tempDir, output); err != nil {
		return fmt.Errorf("failed to create DOCX: %w", err)
	}

	return nil
}

// GetFormat returns the format type
func (p *DocxProcessor) GetFormat() Format {
	return FormatDOCX
}

// ProtectContent protects content during processing
func (p *DocxProcessor) ProtectContent(text string, patternProtector interface{}) string {
	// Use DOCX-specific protector
	return p.protector.Protect(text)
}

// extractDocx extracts DOCX contents to temp directory
func (p *DocxProcessor) extractDocx(data []byte) error {
	reader := bytes.NewReader(data)
	zipReader, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		return err
	}

	for _, file := range zipReader.File {
		if err := p.extractFile(file, p.tempDir); err != nil {
			return fmt.Errorf("failed to extract %s: %w", file.Name, err)
		}
	}

	return nil
}

// extractFile extracts a single file from ZIP
func (p *DocxProcessor) extractFile(file *zip.File, destDir string) error {
	path := filepath.Join(destDir, file.Name)

	// Check for ZipSlip vulnerability
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(destDir)) {
		return fmt.Errorf("invalid file path: %s", file.Name)
	}

	if file.FileInfo().IsDir() {
		return os.MkdirAll(path, file.Mode())
	}

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// Extract file
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	writer, err := os.Create(path)
	if err != nil {
		return err
	}
	defer writer.Close()

	_, err = io.Copy(writer, reader)
	return err
}

// parseDocumentXML parses the main document.xml file
func (p *DocxProcessor) parseDocumentXML() (*WordDocument, error) {
	docPath := filepath.Join(p.tempDir, "word", "document.xml")
	return p.parseXMLFile(docPath)
}

// parseXMLFile parses a WordDocument from XML file
func (p *DocxProcessor) parseXMLFile(path string) (*WordDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var doc WordDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	return &doc, nil
}

// writeXMLFile writes a WordDocument to XML file
func (p *DocxProcessor) writeXMLFile(path string, doc *WordDocument) error {
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}

	// Add XML declaration
	xmlDeclaration := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	data = append([]byte(xmlDeclaration), data...)

	return os.WriteFile(path, data, 0644)
}

// convertToDocument converts WordDocument to Document
func (p *DocxProcessor) convertToDocument(wordDoc *WordDocument) *Document {
	doc := &Document{
		ID:     fmt.Sprintf("docx-%d", time.Now().Unix()),
		Format: FormatDOCX,
		Metadata: DocumentMetadata{
			CreatedAt:    time.Now(),
			CustomFields: make(map[string]interface{}),
		},
		Blocks:    []Block{},
		Resources: make(map[string]Resource),
	}

	// Extract blocks from paragraphs
	for _, para := range wordDoc.Body.Paragraphs {
		if text := p.extractor.ExtractParagraphText(&para); text != "" {
			block := &BaseBlock{
				Type:         BlockTypeParagraph,
				Content:      text,
				Translatable: true,
				Metadata: BlockMetadata{
					Attributes: map[string]interface{}{
						"style": p.getParagraphStyle(&para),
					},
				},
			}
			doc.Blocks = append(doc.Blocks, block)
		}
	}

	// Extract blocks from tables
	for _, table := range wordDoc.Body.Tables {
		p.extractTableBlocks(&table, doc)
	}

	return doc
}

// updateDocumentContent updates WordDocument with translated content
func (p *DocxProcessor) updateDocumentContent(wordDoc *WordDocument, doc *Document) error {
	blockIndex := 0

	// Update paragraphs
	for i := range wordDoc.Body.Paragraphs {
		para := &wordDoc.Body.Paragraphs[i]
		if text := p.extractor.ExtractParagraphText(para); text != "" {
			if blockIndex < len(doc.Blocks) {
				translatedText := doc.Blocks[blockIndex].GetContent()
				// Restore protected content before updating
				restoredText := p.protector.Restore(translatedText)
				p.extractor.UpdateParagraphText(para, restoredText)
				blockIndex++
			}
		}
	}

	// Update tables
	for i := range wordDoc.Body.Tables {
		table := &wordDoc.Body.Tables[i]
		blockIndex = p.updateTableContent(table, doc, blockIndex)
	}

	return nil
}

// getParagraphStyle extracts paragraph style
func (p *DocxProcessor) getParagraphStyle(para *Paragraph) string {
	if para.Properties != nil && para.Properties.Style != nil {
		return para.Properties.Style.Val
	}
	return ""
}

// extractTableBlocks extracts blocks from table
func (p *DocxProcessor) extractTableBlocks(table *Table, doc *Document) {
	for _, row := range table.Rows {
		for _, cell := range row.Cells {
			for _, para := range cell.Paragraphs {
				if text := p.extractor.ExtractParagraphText(&para); text != "" {
					// Apply protection to the text
					protectedText := p.protector.Protect(text)
					
					block := &BaseBlock{
						Type:         BlockTypeParagraph,
						Content:      protectedText,
						Translatable: true,
						Metadata: BlockMetadata{
							Attributes: map[string]interface{}{
								"inTable": true,
								"protected": text != protectedText,
							},
						},
					}
					doc.Blocks = append(doc.Blocks, block)
				}
			}
		}
	}
}

// updateTableContent updates table with translated content
func (p *DocxProcessor) updateTableContent(table *Table, doc *Document, blockIndex int) int {
	for i := range table.Rows {
		row := &table.Rows[i]
		for j := range row.Cells {
			cell := &row.Cells[j]
			for k := range cell.Paragraphs {
				para := &cell.Paragraphs[k]
				if text := p.extractor.ExtractParagraphText(para); text != "" {
					if blockIndex < len(doc.Blocks) {
						translatedText := doc.Blocks[blockIndex].GetContent()
						// Restore protected content before updating
						restoredText := p.protector.Restore(translatedText)
						p.extractor.UpdateParagraphText(para, restoredText)
						blockIndex++
					}
				}
			}
		}
	}
	return blockIndex
}

// createDocx creates a new DOCX file from directory
func (p *DocxProcessor) createDocx(sourceDir string, output io.Writer) error {
	zipWriter := zip.NewWriter(output)
	defer zipWriter.Close()

	// Walk directory and add files
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Skip root directory
		if relPath == "." {
			return nil
		}

		// Convert to forward slashes for ZIP
		relPath = filepath.ToSlash(relPath)

		if info.IsDir() {
			// Create directory entry
			_, err := zipWriter.Create(relPath + "/")
			return err
		}

		// Create file entry
		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		// Copy file content
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

// cleanup removes temporary directory
func (p *DocxProcessor) cleanup() {
	if p.tempDir != "" {
		os.RemoveAll(p.tempDir)
		p.tempDir = ""
	}
}