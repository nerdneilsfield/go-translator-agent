package document

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// TextPackProcessor processes TextPack format documents (.textpack files)
// TextPack is essentially a ZIP-compressed TextBundle
type TextPackProcessor struct {
	opts               ProcessorOptions
	logger             *zap.Logger
	textBundleProcessor *TextBundleProcessor
	tempDir            string
}

// NewTextPackProcessor creates a new TextPack processor
func NewTextPackProcessor(opts ProcessorOptions, logger *zap.Logger) (*TextPackProcessor, error) {
	// Create embedded TextBundle processor
	textBundleProcessor, err := NewTextBundleProcessor(opts, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create TextBundle processor: %w", err)
	}

	return &TextPackProcessor{
		opts:                opts,
		logger:              logger,
		textBundleProcessor: textBundleProcessor,
	}, nil
}

// Parse parses a TextPack file into a Document
func (p *TextPackProcessor) Parse(ctx context.Context, input io.Reader) (*Document, error) {
	// Create temporary directory for extraction
	tempDir, err := os.MkdirTemp("", "textpack-extract-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	p.tempDir = tempDir

	// Read all data from input
	data, err := io.ReadAll(input)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	// Create a temporary file to write the ZIP data
	tempFile := filepath.Join(tempDir, "input.textpack")
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}

	// Extract ZIP contents
	if err := p.extractZip(tempFile, tempDir); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to extract TextPack: %w", err)
	}

	// Find the bundle directory (it might be nested)
	bundleDir, err := p.findBundleDirectory(tempDir)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to find bundle directory: %w", err)
	}

	// Use TextBundle processor to parse the extracted content
	bundlePathReader := strings.NewReader(bundleDir)
	doc, err := p.textBundleProcessor.Parse(ctx, bundlePathReader)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to parse TextBundle: %w", err)
	}

	// Update format to TextPack
	doc.Format = FormatTextPack

	return doc, nil
}

// Process processes the document through translation
func (p *TextPackProcessor) Process(ctx context.Context, doc *Document, translator TranslateFunc) (*Document, error) {
	// Use the TextBundle processor to handle translation
	return p.textBundleProcessor.Process(ctx, doc, translator)
}

// Render renders the document back to TextPack format
func (p *TextPackProcessor) Render(ctx context.Context, doc *Document, output io.Writer) error {
	// Create temporary directory for bundle creation
	tempDir, err := os.MkdirTemp("", "textpack-render-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create bundle directory inside temp
	bundleDir := filepath.Join(tempDir, "bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return fmt.Errorf("failed to create bundle directory: %w", err)
	}

	// Use TextBundle processor to render to the bundle directory
	bundlePathBuilder := &strings.Builder{}
	bundlePathBuilder.WriteString(bundleDir)
	if err := p.textBundleProcessor.Render(ctx, doc, bundlePathBuilder); err != nil {
		return fmt.Errorf("failed to render TextBundle: %w", err)
	}

	// Create ZIP file
	zipFile := filepath.Join(tempDir, "output.textpack")
	if err := p.createZip(bundleDir, zipFile); err != nil {
		return fmt.Errorf("failed to create TextPack: %w", err)
	}

	// Read ZIP file and write to output
	zipData, err := os.ReadFile(zipFile)
	if err != nil {
		return fmt.Errorf("failed to read ZIP file: %w", err)
	}

	if _, err := output.Write(zipData); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

// GetFormat returns the format type
func (p *TextPackProcessor) GetFormat() Format {
	return FormatTextPack
}

// ProtectContent protects content during processing
func (p *TextPackProcessor) ProtectContent(text string, patternProtector interface{}) string {
	// Use TextBundle protector
	return p.textBundleProcessor.ProtectContent(text, patternProtector)
}

// Cleanup removes temporary files
func (p *TextPackProcessor) Cleanup() {
	if p.tempDir != "" {
		os.RemoveAll(p.tempDir)
		p.tempDir = ""
	}
}

// extractZip extracts a ZIP file to a directory
func (p *TextPackProcessor) extractZip(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(destDir, file.Name)

		// Check for ZipSlip vulnerability
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(destDir)) {
			return fmt.Errorf("invalid file path: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.Mode())
			continue
		}

		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		// Extract file
		fileReader, err := file.Open()
		if err != nil {
			return err
		}
		defer fileReader.Close()

		targetFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}
		defer targetFile.Close()

		_, err = io.Copy(targetFile, fileReader)
		if err != nil {
			return err
		}
	}

	return nil
}

// createZip creates a ZIP file from a directory
func (p *TextPackProcessor) createZip(sourceDir, targetPath string) error {
	zipFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk the directory and add files to ZIP
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(filepath.Dir(sourceDir), path)
		if err != nil {
			return err
		}

		// Convert to forward slashes for ZIP
		relPath = filepath.ToSlash(relPath)

		if info.IsDir() {
			// Add directory entry
			_, err := zipWriter.Create(relPath + "/")
			return err
		}

		// Create file entry
		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		// Open and copy file
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

// findBundleDirectory finds the TextBundle directory within the extracted content
func (p *TextPackProcessor) findBundleDirectory(rootDir string) (string, error) {
	// First check if the root directory itself is a bundle
	if _, err := os.Stat(filepath.Join(rootDir, "info.json")); err == nil {
		return rootDir, nil
	}

	// Otherwise, look for a bundle directory one level deep
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			path := filepath.Join(rootDir, entry.Name())
			if _, err := os.Stat(filepath.Join(path, "info.json")); err == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("no valid TextBundle found in archive")
}