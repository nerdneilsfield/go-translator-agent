package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type PredefinedTranslation struct {
	SourceLang   string            `toml:"source_lang"`
	TargetLang   string            `toml:"target_lang"`
	Translations map[string]string `toml:"translations"`
}

func NewPredefinedTranslation(sourceLang, targetLang string, translations map[string]string) *PredefinedTranslation {
	return &PredefinedTranslation{
		SourceLang:   sourceLang,
		TargetLang:   targetLang,
		Translations: translations,
	}
}

func LoadPredefinedTranslations(path string) (*PredefinedTranslation, error) {
	// check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("predefined translations file not found: %s", path)
	}

	// load toml file
	translations := &PredefinedTranslation{}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read predefined translations file: %w", err)
	}
	if err := toml.Unmarshal(content, translations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal predefined translations: %w", err)
	}
	if translations.SourceLang == "" || translations.TargetLang == "" {
		return nil, fmt.Errorf("predefined translations file is missing source_lang or target_lang")
	}
	return translations, nil
}
