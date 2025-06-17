package translator

import (
	"testing"
)

func TestCalculateSimilarity(t *testing.T) {
	bt := &BatchTranslator{}

	tests := []struct {
		name    string
		text1   string
		text2   string
		wantMin float64
		wantMax float64
	}{
		{
			name:    "identical texts",
			text1:   "Hello world",
			text2:   "Hello world",
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name:    "completely different",
			text1:   "Hello world",
			text2:   "你好世界",
			wantMin: 0.0,
			wantMax: 0.3,
		},
		{
			name:    "english to chinese translation",
			text1:   "We propose a new non-recursive structure",
			text2:   "我们提出了一种新的非递归结构",
			wantMin: 0.0,
			wantMax: 0.3,
		},
		{
			name:    "similar but not translated",
			text1:   "Hello world",
			text2:   "Hello world!",
			wantMin: 0.8,
			wantMax: 1.0,
		},
		{
			name:    "empty strings",
			text1:   "",
			text2:   "",
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name:    "one empty string",
			text1:   "Hello",
			text2:   "",
			wantMin: 0.0,
			wantMax: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bt.calculateSimilarity(tt.text1, tt.text2)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("calculateSimilarity() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSimilarityThreshold(t *testing.T) {
	bt := &BatchTranslator{}

	// 测试阈值的合理性
	tests := []struct {
		name        string
		original    string
		translated  string
		shouldPass  bool
		description string
	}{
		{
			name:        "good english to chinese translation",
			original:    "Hello world",
			translated:  "你好世界",
			shouldPass:  true,
			description: "proper translation should pass",
		},
		{
			name:        "untranslated text",
			original:    "Hello world",
			translated:  "Hello world",
			shouldPass:  false,
			description: "untranslated text should fail",
		},
		{
			name:        "minor changes only",
			original:    "Hello world",
			translated:  "Hello world.",
			shouldPass:  false,
			description: "minor punctuation changes should fail",
		},
		{
			name:        "mixed language paragraph - english original",
			original:    "We propose a new non-recursive and memory-efficient structure OAVS",
			translated:  "我们提出了一种新的非递归且节省内存的结构 OAVS",
			shouldPass:  true,
			description: "translation with preserved technical terms should pass",
		},
	}

	threshold := 0.8

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := bt.calculateSimilarity(tt.original, tt.translated)
			passed := similarity < threshold

			if passed != tt.shouldPass {
				t.Errorf("%s: similarity=%v, threshold=%v, passed=%v, expected=%v",
					tt.description, similarity, threshold, passed, tt.shouldPass)
			}
		})
	}
}
