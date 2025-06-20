package document

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// TextBundleProcessor processes TextBundle format documents
type TextBundleProcessor struct {
	opts               ProcessorOptions
	logger             *zap.Logger
	markdownProcessor  *MarkdownProcessor
	bundlePath         string
	bundleInfo         *BundleInfo
	assetProtector     *AssetProtector
}

// AssetProtector protects asset references during translation
type AssetProtector struct {
	placeholders map[string]string
	counter      int
}

// NewAssetProtector creates a new asset protector
func NewAssetProtector() *AssetProtector {
	return &AssetProtector{
		placeholders: make(map[string]string),
		counter:      0,
	}
}

// Protect replaces asset references with placeholders
func (ap *AssetProtector) Protect(content string) string {
	// Pattern to match asset references in markdown
	// Matches: ![alt](assets/...) and [text](assets/...)
	patterns := []string{
		`!\[([^\]]*)\]\(assets/([^)]+)\)`, // Images
		`\[([^\]]*)\]\(assets/([^)]+)\)`,  // Links
	}

	protected := content
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		protected = re.ReplaceAllStringFunc(protected, func(match string) string {
			ap.counter++
			placeholder := fmt.Sprintf("@@ASSET_%d@@", ap.counter)
			ap.placeholders[placeholder] = match
			return placeholder
		})
	}

	return protected
}

// Restore replaces placeholders back with original asset references
func (ap *AssetProtector) Restore(content string) string {
	restored := content
	for placeholder, original := range ap.placeholders {
		restored = strings.ReplaceAll(restored, placeholder, original)
	}
	return restored
}

// NewTextBundleProcessor creates a new TextBundle processor
func NewTextBundleProcessor(opts ProcessorOptions, logger *zap.Logger) (*TextBundleProcessor, error) {
	// Create embedded markdown processor
	markdownProcessor, err := NewMarkdownProcessor(opts, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create markdown processor: %w", err)
	}

	return &TextBundleProcessor{
		opts:              opts,
		logger:            logger,
		markdownProcessor: markdownProcessor,
		assetProtector:    NewAssetProtector(),
	}, nil
}

// Parse parses a TextBundle directory into a Document
func (p *TextBundleProcessor) Parse(ctx context.Context, input io.Reader) (*Document, error) {
	// Read all content from input (expecting a file path)
	pathBytes, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	// The input should be a path to the .textbundle directory
	bundlePath := strings.TrimSpace(string(pathBytes))
	p.bundlePath = bundlePath

	// Validate it's a directory
	info, err := os.Stat(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to access bundle: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("bundle path is not a directory: %s", bundlePath)
	}

	// Load bundle info
	bundleInfo, err := LoadBundleInfo(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load bundle info: %w", err)
	}
	p.bundleInfo = bundleInfo

	// Read text file
	textPath := filepath.Join(bundlePath, bundleInfo.GetTextFileName())
	textContent, err := os.ReadFile(textPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read text file: %w", err)
	}

	// Protect asset references
	protectedContent := p.assetProtector.Protect(string(textContent))

	// Parse as markdown (or other format based on type)
	reader := strings.NewReader(protectedContent)
	doc, err := p.markdownProcessor.Parse(ctx, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse text content: %w", err)
	}

	// Update document metadata
	doc.Format = FormatTextBundle
	doc.Metadata.CustomFields = map[string]interface{}{
		"bundlePath": bundlePath,
		"bundleInfo": bundleInfo,
	}

	// Load assets as resources
	assetsPath := filepath.Join(bundlePath, "assets")
	if _, err := os.Stat(assetsPath); err == nil {
		if err := p.loadAssets(doc, assetsPath); err != nil {
			p.logger.Warn("failed to load some assets", zap.Error(err))
		}
	}

	return doc, nil
}

// Process processes the document through translation
func (p *TextBundleProcessor) Process(ctx context.Context, doc *Document, translator TranslateFunc) (*Document, error) {
	// Use the markdown processor to handle translation
	translatedDoc, err := p.markdownProcessor.Process(ctx, doc, translator)
	if err != nil {
		return nil, fmt.Errorf("failed to process document: %w", err)
	}

	// The asset references are already protected, so they won't be translated
	return translatedDoc, nil
}

// Render renders the document back to TextBundle format
func (p *TextBundleProcessor) Render(ctx context.Context, doc *Document, output io.Writer) error {
	// Get the output path from the writer
	var outputPath string
	
	// Try to get path from custom writer or create temp directory
	if pathWriter, ok := output.(*PathWriter); ok {
		outputPath = pathWriter.Path
	} else {
		// For standard io.Writer, we need to create a temporary bundle
		// and then write a marker to indicate where it was created
		tempDir, err := os.MkdirTemp("", "textbundle-output-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		outputPath = tempDir
	}

	// Create output bundle directory
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output bundle: %w", err)
	}

	// Render markdown content
	var contentBuilder strings.Builder
	if err := p.markdownProcessor.Render(ctx, doc, &contentBuilder); err != nil {
		return fmt.Errorf("failed to render content: %w", err)
	}

	// Restore asset references
	finalContent := p.assetProtector.Restore(contentBuilder.String())

	// Write text file
	textFileName := "text.md"
	if p.bundleInfo != nil {
		textFileName = p.bundleInfo.GetTextFileName()
	}
	textPath := filepath.Join(outputPath, textFileName)
	if err := os.WriteFile(textPath, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("failed to write text file: %w", err)
	}

	// Write info.json
	info := p.bundleInfo
	if info == nil {
		// Create default info if not available
		info = &BundleInfo{
			Version: 2,
			Type:    "net.daringfireball.markdown",
		}
	}

	// Add translation metadata
	if info.ExtensionData == nil {
		info.ExtensionData = make(map[string]interface{})
	}
	info.ExtensionData["translatedAt"] = time.Now().Format(time.RFC3339)
	info.ExtensionData["translatedBy"] = "go-translator-agent"

	if err := SaveBundleInfo(outputPath, info); err != nil {
		return fmt.Errorf("failed to save bundle info: %w", err)
	}

	// Copy assets if they exist
	if p.bundlePath != "" {
		srcAssetsPath := filepath.Join(p.bundlePath, "assets")
		if _, err := os.Stat(srcAssetsPath); err == nil {
			dstAssetsPath := filepath.Join(outputPath, "assets")
			if err := p.copyAssets(srcAssetsPath, dstAssetsPath); err != nil {
				p.logger.Warn("failed to copy assets", zap.Error(err))
			}
		}
	}

	// If using standard writer, write a marker indicating the bundle location
	if _, ok := output.(*PathWriter); !ok {
		marker := fmt.Sprintf("[TextBundle saved to: %s]\n", outputPath)
		if _, err := output.Write([]byte(marker)); err != nil {
			return fmt.Errorf("failed to write output marker: %w", err)
		}
	}

	return nil
}

// GetFormat returns the format type
func (p *TextBundleProcessor) GetFormat() Format {
	return FormatTextBundle
}

// ProtectContent protects content during processing
func (p *TextBundleProcessor) ProtectContent(text string, patternProtector interface{}) string {
	// Use markdown protector for text content
	return p.markdownProcessor.ProtectContent(text, patternProtector)
}

// loadAssets loads assets from the bundle into document resources
func (p *TextBundleProcessor) loadAssets(doc *Document, assetsPath string) error {
	return filepath.Walk(assetsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Get relative path from assets directory
		relPath, err := filepath.Rel(assetsPath, path)
		if err != nil {
			return err
		}

		// Read asset data
		data, err := os.ReadFile(path)
		if err != nil {
			p.logger.Warn("failed to read asset", zap.String("path", path), zap.Error(err))
			return nil // Continue with other assets
		}

		// Determine resource type
		resourceType := ResourceTypeOther
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp":
			resourceType = ResourceTypeImage
		case ".css":
			resourceType = ResourceTypeStylesheet
		case ".ttf", ".otf", ".woff", ".woff2":
			resourceType = ResourceTypeFont
		}

		// Add to resources
		resourceID := filepath.ToSlash(relPath)
		doc.Resources[resourceID] = Resource{
			ID:   resourceID,
			Type: resourceType,
			Data: data,
			URL:  filepath.Join("assets", relPath),
		}

		return nil
	})
}

// copyAssets copies assets from source to destination
func (p *TextBundleProcessor) copyAssets(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			// Create directory
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		// Create destination directory if needed
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}