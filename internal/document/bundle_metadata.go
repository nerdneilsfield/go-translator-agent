package document

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BundleInfo represents the info.json structure in a TextBundle
type BundleInfo struct {
	Version            int                    `json:"version"`
	Type               string                 `json:"type,omitempty"`
	Transient          bool                   `json:"transient,omitempty"`
	CreatorURL         string                 `json:"creatorURL,omitempty"`
	CreatorIdentifier  string                 `json:"creatorIdentifier,omitempty"`
	SourceURL          string                 `json:"sourceURL,omitempty"`
	ExtensionData      map[string]interface{} `json:"-"`
}

// UnmarshalJSON implements custom unmarshaling to preserve extension data
func (b *BundleInfo) UnmarshalJSON(data []byte) error {
	// First unmarshal into a map to get all fields
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Extract known fields
	if v, ok := raw["version"].(float64); ok {
		b.Version = int(v)
	}
	if v, ok := raw["type"].(string); ok {
		b.Type = v
	}
	if v, ok := raw["transient"].(bool); ok {
		b.Transient = v
	}
	if v, ok := raw["creatorURL"].(string); ok {
		b.CreatorURL = v
	}
	if v, ok := raw["creatorIdentifier"].(string); ok {
		b.CreatorIdentifier = v
	}
	if v, ok := raw["sourceURL"].(string); ok {
		b.SourceURL = v
	}

	// Store extension data (fields we don't recognize)
	b.ExtensionData = make(map[string]interface{})
	knownFields := map[string]bool{
		"version":           true,
		"type":              true,
		"transient":         true,
		"creatorURL":        true,
		"creatorIdentifier": true,
		"sourceURL":         true,
	}

	for k, v := range raw {
		if !knownFields[k] {
			b.ExtensionData[k] = v
		}
	}

	return nil
}

// MarshalJSON implements custom marshaling to include extension data
func (b *BundleInfo) MarshalJSON() ([]byte, error) {
	// Create a map with all fields
	data := make(map[string]interface{})

	// Add known fields
	data["version"] = b.Version
	if b.Type != "" {
		data["type"] = b.Type
	}
	if b.Transient {
		data["transient"] = b.Transient
	}
	if b.CreatorURL != "" {
		data["creatorURL"] = b.CreatorURL
	}
	if b.CreatorIdentifier != "" {
		data["creatorIdentifier"] = b.CreatorIdentifier
	}
	if b.SourceURL != "" {
		data["sourceURL"] = b.SourceURL
	}

	// Add extension data
	for k, v := range b.ExtensionData {
		data[k] = v
	}

	return json.Marshal(data)
}

// LoadBundleInfo loads and parses info.json from a TextBundle directory
func LoadBundleInfo(bundlePath string) (*BundleInfo, error) {
	infoPath := filepath.Join(bundlePath, "info.json")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read info.json: %w", err)
	}

	var info BundleInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse info.json: %w", err)
	}

	// Validate required fields
	if info.Version == 0 {
		return nil, fmt.Errorf("invalid info.json: version field is required")
	}

	// Set default type if not specified
	if info.Type == "" {
		info.Type = "net.daringfireball.markdown"
	}

	return &info, nil
}

// SaveBundleInfo saves BundleInfo to info.json in a TextBundle directory
func SaveBundleInfo(bundlePath string, info *BundleInfo) error {
	infoPath := filepath.Join(bundlePath, "info.json")
	
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal info.json: %w", err)
	}

	if err := os.WriteFile(infoPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write info.json: %w", err)
	}

	return nil
}

// GetTextFileExtension returns the expected text file extension based on the type
func (b *BundleInfo) GetTextFileExtension() string {
	switch b.Type {
	case "net.daringfireball.markdown":
		return ".md"
	case "public.plain-text":
		return ".txt"
	case "org.textbundle.html":
		return ".html"
	default:
		// Default to markdown
		return ".md"
	}
}

// GetTextFileName returns the expected text file name
func (b *BundleInfo) GetTextFileName() string {
	return "text" + b.GetTextFileExtension()
}