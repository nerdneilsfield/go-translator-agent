package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/nerdneilsfield/go-translator-agent/internal/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParallelTranslationPerformance 测试并行翻译性能
func TestParallelTranslationPerformance(t *testing.T) {
	// 创建测试配置
	tempDir := t.TempDir()
	progressPath := tempDir + "/progress"
	
	// 创建包含多个块的长文本（每个块约500字符，共10个块）
	longText := generateLongTestText(10, 500)
	
	t.Run("Serial vs Parallel Performance", func(t *testing.T) {
		// 测试串行翻译（并发数=1）
		serialTime := measureTranslationTime(t, longText, 1, progressPath+"_serial")
		
		// 测试并行翻译（并发数=4）
		parallelTime := measureTranslationTime(t, longText, 4, progressPath+"_parallel")
		
		t.Logf("串行翻译时间 (concurrency=1): %v", serialTime)
		t.Logf("并行翻译时间 (concurrency=4): %v", parallelTime)
		
		// 由于是模拟翻译器，性能差异可能不明显
		// 但我们可以验证系统正常工作
		speedup := float64(serialTime) / float64(parallelTime)
		t.Logf("加速比: %.2fx", speedup)
		
		// 验证并行翻译至少不比串行翻译慢很多
		assert.LessOrEqual(t, float64(parallelTime)/float64(serialTime), 2.0, "并行翻译不应该比串行翻译慢太多")
	})
	
	t.Run("Concurrency Scaling", func(t *testing.T) {
		// 测试不同并发级别的性能
		concurrencyLevels := []int{1, 2, 4, 8}
		times := make([]time.Duration, len(concurrencyLevels))
		
		for i, concurrency := range concurrencyLevels {
			times[i] = measureTranslationTime(t, longText, concurrency, fmt.Sprintf("%s_conc%d", progressPath, concurrency))
			t.Logf("并发数 %d: %v", concurrency, times[i])
		}
		
		// 验证随着并发数增加，翻译时间应该减少（或至少不显著增加）
		for i := 1; i < len(times); i++ {
			// 允许一些波动，但大体趋势应该是时间减少
			ratio := float64(times[i]) / float64(times[0])
			t.Logf("并发数 %d 相对于串行的时间比率: %.2f", concurrencyLevels[i], ratio)
			assert.LessOrEqual(t, ratio, 1.2, "高并发应该不会显著增加翻译时间")
		}
	})
}

// measureTranslationTime 测量翻译时间
func measureTranslationTime(t *testing.T, text string, concurrency int, progressPath string) time.Duration {
	// 创建配置
	cfg := createParallelTestConfig(concurrency, progressPath)
	log := logger.NewLogger(false)
	
	// 创建翻译协调器
	coordinator, err := translator.NewTranslationCoordinator(cfg, log, progressPath)
	require.NoError(t, err)
	
	// 开始计时
	start := time.Now()
	
	// 执行翻译
	ctx := context.Background()
	result, err := coordinator.TranslateText(ctx, text)
	
	// 结束计时
	elapsed := time.Since(start)
	
	// 验证翻译结果
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	
	return elapsed
}

// createParallelTestConfig 创建并行测试配置
func createParallelTestConfig(concurrency int, progressPath string) *config.Config {
	cfg := createTestConfig(progressPath)
	cfg.Concurrency = concurrency
	cfg.ChunkSize = 500 // 较小的分块确保会产生多个块
	return cfg
}

// generateLongTestText 生成长测试文本
func generateLongTestText(numChunks, chunkSize int) string {
	var chunks []string
	
	baseTexts := []string{
		"Machine learning is a subset of artificial intelligence that enables computers to learn and improve from experience without being explicitly programmed.",
		"Deep learning uses neural networks with multiple layers to model and understand complex patterns in data.",
		"Natural language processing helps computers understand, interpret, and generate human language in a valuable way.",
		"Computer vision enables machines to interpret and make decisions based on visual data from the world around them.",
		"Reinforcement learning is a type of machine learning where agents learn to make decisions by taking actions in an environment.",
		"Data science combines statistics, mathematics, programming, and domain expertise to extract insights from data.",
		"Big data refers to extremely large datasets that require specialized tools and techniques to process and analyze.",
		"Cloud computing provides on-demand access to computing resources over the internet without direct active management.",
		"Cybersecurity protects digital systems, networks, and data from unauthorized access, use, disclosure, or destruction.",
		"Software engineering applies engineering principles to the design, development, maintenance, testing, and evaluation of software.",
	}
	
	for i := 0; i < numChunks; i++ {
		baseText := baseTexts[i%len(baseTexts)]
		// 扩展文本以达到目标大小
		chunk := baseText
		for len(chunk) < chunkSize {
			chunk += " " + baseText
		}
		chunks = append(chunks, chunk[:chunkSize])
	}
	
	return strings.Join(chunks, "\n\n")
}

// TestParallelTranslationCorrectness 测试并行翻译的正确性
func TestParallelTranslationCorrectness(t *testing.T) {
	tempDir := t.TempDir()
	progressPath := tempDir + "/progress"
	
	// 创建测试文本，包含多个可识别的块
	testText := createIdentifiableTestText()
	
	// 测试串行和并行翻译的结果是否一致
	cfg1 := createParallelTestConfig(1, progressPath+"_serial")
	cfg2 := createParallelTestConfig(4, progressPath+"_parallel")
	
	log := logger.NewLogger(false)
	
	coordinator1, err := translator.NewTranslationCoordinator(cfg1, log, progressPath+"_serial")
	require.NoError(t, err)
	
	coordinator2, err := translator.NewTranslationCoordinator(cfg2, log, progressPath+"_parallel")
	require.NoError(t, err)
	
	ctx := context.Background()
	
	// 执行串行翻译
	result1, err := coordinator1.TranslateText(ctx, testText)
	require.NoError(t, err)
	
	// 执行并行翻译
	result2, err := coordinator2.TranslateText(ctx, testText)
	require.NoError(t, err)
	
	// 验证结果相同（因为是模拟翻译器，结果应该完全一致）
	assert.Equal(t, result1, result2, "串行和并行翻译应该产生相同的结果")
	
	// 验证所有块都被翻译了
	expectedChunks := []string{"Chunk 1", "Chunk 2", "Chunk 3", "Chunk 4", "Chunk 5"}
	for _, chunk := range expectedChunks {
		assert.Contains(t, result1, "Translated: "+chunk, "应该包含翻译后的块: "+chunk)
		assert.Contains(t, result2, "Translated: "+chunk, "应该包含翻译后的块: "+chunk)
	}
	
	t.Logf("串行翻译结果长度: %d", len(result1))
	t.Logf("并行翻译结果长度: %d", len(result2))
}

// createIdentifiableTestText 创建可识别的测试文本
func createIdentifiableTestText() string {
	chunks := []string{
		"Chunk 1: This is the first part of our test document. It contains information about machine learning and artificial intelligence.",
		"Chunk 2: This section discusses deep learning methodologies and neural network architectures used in modern AI systems.",
		"Chunk 3: Here we explore natural language processing techniques and their applications in text analysis and generation.",
		"Chunk 4: This part covers computer vision algorithms and their role in image recognition and processing tasks.",
		"Chunk 5: Finally, we examine reinforcement learning approaches and their effectiveness in decision-making problems.",
	}
	
	return strings.Join(chunks, "\n\n")
}

// TestChunkProcessingOrder 测试块处理顺序
func TestChunkProcessingOrder(t *testing.T) {
	tempDir := t.TempDir()
	progressPath := tempDir + "/progress"
	
	// 创建有序的测试文本
	testText := createOrderedTestText()
	
	cfg := createParallelTestConfig(4, progressPath)
	log := logger.NewLogger(false)
	
	coordinator, err := translator.NewTranslationCoordinator(cfg, log, progressPath)
	require.NoError(t, err)
	
	ctx := context.Background()
	
	// 执行翻译
	result, err := coordinator.TranslateText(ctx, testText)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	
	// 验证输出的顺序正确（即使并行处理，输出应该保持原始顺序）
	lines := strings.Split(result, "\n")
	var translatedLines []string
	for _, line := range lines {
		if strings.Contains(line, "Translated: Part") {
			translatedLines = append(translatedLines, line)
		}
	}
	
	// 验证顺序
	expectedOrder := []string{"Part 1", "Part 2", "Part 3", "Part 4", "Part 5"}
	for i, expected := range expectedOrder {
		if i < len(translatedLines) {
			assert.Contains(t, translatedLines[i], expected, 
				fmt.Sprintf("第 %d 行应该包含 %s", i+1, expected))
		}
	}
	
	t.Logf("翻译后的行数: %d", len(translatedLines))
	for i, line := range translatedLines {
		t.Logf("第 %d 行: %s", i+1, line)
	}
}

// createOrderedTestText 创建有序的测试文本
func createOrderedTestText() string {
	parts := []string{
		"Part 1: Introduction to the concepts and basic principles that will be covered in this comprehensive guide.",
		"Part 2: Detailed explanation of the fundamental theories and their practical applications in real-world scenarios.",
		"Part 3: Advanced techniques and methodologies that build upon the foundation established in previous sections.",
		"Part 4: Case studies and examples that demonstrate the effectiveness of the approaches discussed earlier.",
		"Part 5: Conclusion and future directions for research and development in this rapidly evolving field.",
	}
	
	return strings.Join(parts, "\n\n")
}