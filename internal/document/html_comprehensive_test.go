package document

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

// TestHTMLPlaceholderManager 测试HTML占位符管理器
func TestHTMLPlaceholderManager(t *testing.T) {
	manager := NewHTMLPlaceholderManager()

	// 测试基本保护和恢复
	content := "<script>alert('test')</script>"
	placeholder := manager.Protect(content)

	if placeholder == content {
		t.Error("Protect should return a placeholder, not original content")
	}

	if !strings.HasPrefix(placeholder, "@@PROTECTED_") {
		t.Error("Placeholder should start with @@PROTECTED_")
	}

	restored := manager.Restore(placeholder)
	if restored != content {
		t.Errorf("Restore failed: expected %s, got %s", content, restored)
	}

	// 测试属性保护
	tagPlaceholder := manager.ProtectWithAttributes("img", ` src="test.jpg" alt="test"`, "")
	if !manager.HasPlaceholders(tagPlaceholder) {
		t.Error("ProtectWithAttributes should create a placeholder")
	}

	// 测试计数和清理
	count := manager.GetPlaceholderCount()
	if count != 2 {
		t.Errorf("Expected 2 placeholders, got %d", count)
	}

	manager.Clear()
	if manager.GetPlaceholderCount() != 0 {
		t.Error("Clear should remove all placeholders")
	}
}

// TestHTMLProtector 测试HTML保护器
func TestHTMLProtector(t *testing.T) {
	protector := NewHTMLProtector()

	// 模拟PatternProtector
	mockPatternProtector := &MockPatternProtector{}

	testHTML := `
		<html>
		<head>
			<script src="test.js"></script>
			<style>body { color: red; }</style>
		</head>
		<body>
			<h1>Title</h1>
			<p>This is a paragraph.</p>
			<a class="page" id="p1"/>
			<nav>Navigation</nav>
			<svg><circle r="5"/></svg>
		</body>
		</html>
	`

	protected := protector.ProtectContent(testHTML, mockPatternProtector)

	// 验证保护模式被调用
	if len(mockPatternProtector.ProtectedPatterns) == 0 {
		t.Error("No protection patterns were applied")
	}

	// 验证特定模式被保护
	expectedPatterns := []string{
		"Script tags",
		"Style tags",
		"SVG and math formulas",
		"Page anchors and navigation elements",
	}

	for _, pattern := range expectedPatterns {
		found := false
		for _, protected := range mockPatternProtector.ProtectedPatterns {
			if strings.Contains(protected, pattern) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Pattern %s was not protected", pattern)
		}
	}
}

// TestHTMLSmartExtractor 测试智能HTML节点提取器
func TestHTMLSmartExtractor(t *testing.T) {
	logger := zap.NewNop()
	options := DefaultSmartExtractorOptions()
	extractor := NewHTMLSmartExtractor(logger, options)

	testHTML := `
		<html>
		<body>
			<h1>Main Title</h1>
			<p>This is a paragraph with <strong>bold text</strong>.</p>
			<img src="test.jpg" alt="Test image" title="Image title"/>
			<div translate="no">Do not translate this</div>
			<svg><circle r="5"/></svg>
			<p>Another paragraph.</p>
		</body>
		</html>
	`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(testHTML))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	nodes, err := extractor.ExtractTranslatableNodes(doc)
	if err != nil {
		t.Fatalf("Failed to extract nodes: %v", err)
	}

	// 验证提取的节点
	if len(nodes) == 0 {
		t.Error("No nodes were extracted")
	}

	// 检查是否正确跳过了translate="no"的元素
	for _, node := range nodes {
		if strings.Contains(node.Text, "Do not translate this") {
			t.Error("Node with translate='no' should be skipped")
		}
	}

	// 检查是否提取了属性
	hasAttributes := false
	for _, node := range nodes {
		if node.IsAttribute {
			hasAttributes = true
			break
		}
	}
	if !hasAttributes {
		t.Error("No attributes were extracted")
	}

	// 验证上下文信息
	for _, node := range nodes {
		if node.ParentTag == "" {
			t.Error("Parent tag should be set")
		}
	}
}

// TestHTMLAttributeTranslator 测试HTML属性翻译器
func TestHTMLAttributeTranslator(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultAttributeTranslationConfig()
	translator := NewHTMLAttributeTranslator(logger, config)

	testHTML := `
		<html>
		<body>
			<img src="test.jpg" alt="Test image" title="Image title"/>
			<input type="text" placeholder="Enter your name"/>
			<button type="submit" value="Submit">Submit</button>
			<a href="#" title="Link title">Link</a>
			<div translate="no" title="Do not translate">Content</div>
		</body>
		</html>
	`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(testHTML))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	attributes, err := translator.ExtractTranslatableAttributes(doc)
	if err != nil {
		t.Fatalf("Failed to extract attributes: %v", err)
	}

	if len(attributes) == 0 {
		t.Error("No attributes were extracted")
	}

	// 验证提取的属性类型
	hasAlt := false
	hasPlaceholder := false
	for _, attr := range attributes {
		if attr.AttributeName == "alt" {
			hasAlt = true
		}
		if attr.AttributeName == "placeholder" {
			hasPlaceholder = true
		}
	}

	if !hasAlt {
		t.Error("Alt attribute should be extracted")
	}
	if !hasPlaceholder {
		t.Error("Placeholder attribute should be extracted")
	}

	// 测试批量翻译
	mockBatchTranslator := func(ctx context.Context, texts []string) ([]string, error) {
		results := make([]string, len(texts))
		for i, text := range texts {
			results[i] = "TRANSLATED: " + text
		}
		return results, nil
	}

	err = translator.BatchTranslateAttributes(context.Background(), attributes, mockBatchTranslator)
	if err != nil {
		t.Fatalf("Batch translation failed: %v", err)
	}

	// 验证翻译结果
	for _, attr := range attributes {
		if attr.CanTranslate && attr.TranslatedValue == "" {
			t.Error("Translatable attribute should have translated value")
		}
		if attr.TranslatedValue != "" && !strings.HasPrefix(attr.TranslatedValue, "TRANSLATED:") {
			t.Error("Translation result should have expected prefix")
		}
	}
}

// TestHTMLRetryManager 测试HTML重试管理器
func TestHTMLRetryManager(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultHTMLRetryConfig()
	config.MaxRetries = 2
	config.RetryDelay = time.Millisecond * 10

	retryManager := NewHTMLRetryManager(logger, config)

	// 创建测试节点
	node := &ExtractableNode{
		Path: "/test/node",
		Text: "Test text",
	}

	// 测试失败记录
	testError := &MockTranslationError{message: "network timeout"}
	context := &HTMLTranslationContext{
		ElementTag: "p",
		SiblingsBefore: []string{"Previous text"},
	}

	retryManager.RecordFailure(node, testError, context)

	// 验证应该重试
	if !retryManager.ShouldRetry(node.Path) {
		t.Error("Should retry after first failure")
	}

	// 记录更多失败
	retryManager.RecordFailure(node, testError, context)
	retryManager.RecordFailure(node, testError, context)

	// 验证达到最大重试次数后不再重试
	if retryManager.ShouldRetry(node.Path) {
		t.Error("Should not retry after reaching max retries")
	}

	// 测试成功记录
	retryManager.RecordSuccess(node.Path, "Translated text")

	// 验证统计信息
	stats := retryManager.GetRetryStatistics()
	if stats["totalRetryNodes"].(int) != 0 {
		t.Error("Successful node should be removed from retry history")
	}
}

// TestHTMLPerformanceOptimizer 测试HTML性能优化器
func TestHTMLPerformanceOptimizer(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultPerformanceConfig()
	config.EnableCaching = true
	config.EnablePrefiltering = true
	config.EnableDeduplication = true

	optimizer := NewHTMLPerformanceOptimizer(logger, config)

	// 创建测试节点
	nodes := []*ExtractableNode{
		{Path: "/node1", Text: "Hello world", ParentTag: "p"},
		{Path: "/node2", Text: "Hello world", ParentTag: "p"}, // 重复
		{Path: "/node3", Text: "123", ParentTag: "span"},      // 数字
		{Path: "/node4", Text: "!", ParentTag: "span"},        // 标点
		{Path: "/node5", Text: "Real content", ParentTag: "h1"},
	}

	// 模拟翻译器
	mockTranslator := func(ctx context.Context, text string) (string, error) {
		return "TRANSLATED: " + text, nil
	}

	// 测试优化翻译
	results, err := optimizer.OptimizeTranslation(context.Background(), nodes, mockTranslator)
	if err != nil {
		t.Fatalf("Optimization failed: %v", err)
	}

	// 验证结果
	if len(results) == 0 {
		t.Error("No results returned")
	}

	// 验证缓存工作
	results2, err := optimizer.OptimizeTranslation(context.Background(), nodes, mockTranslator)
	if err != nil {
		t.Fatalf("Second optimization failed: %v", err)
	}

	metrics := optimizer.GetMetrics()
	if metrics.CacheHits == 0 {
		t.Error("Cache should have hits on second run")
	}

	// 验证指标
	if metrics.TotalJobs == 0 {
		t.Error("Metrics should record total jobs")
	}
	if metrics.CompletedJobs == 0 {
		t.Error("Metrics should record completed jobs")
	}

	t.Logf("Performance metrics: %+v", metrics)
	t.Logf("Results count: %d", len(results2))
}

// TestHTMLEnhancedProcessor 测试增强HTML处理器集成
func TestHTMLEnhancedProcessor(t *testing.T) {
	logger := zap.NewNop()
	protector := NewHTMLProtector()
	options := DefaultEnhancedProcessorOptions()
	options.EnableRetry = false // 简化测试

	processor := NewHTMLEnhancedProcessor(logger, protector, options)

	testHTML := `<!DOCTYPE html>
		<html>
		<head>
			<title>Test Page</title>
		</head>
		<body>
			<h1>Main Title</h1>
			<p>This is a test paragraph.</p>
			<img src="test.jpg" alt="Test image"/>
			<div>Another content block.</div>
		</body>
		</html>
	`

	// 解析
	doc, err := processor.Parse(context.Background(), strings.NewReader(testHTML))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// 验证文档结构
	if doc.XMLDeclaration != "" {
		t.Logf("XML Declaration: %s", doc.XMLDeclaration)
	}
	if doc.DOCTYPE == "" {
		t.Error("DOCTYPE should be preserved")
	}

	// 模拟翻译器
	mockTranslator := func(ctx context.Context, text string) (string, error) {
		return "翻译: " + text, nil
	}

	// 处理翻译
	processedDoc, err := processor.Process(context.Background(), doc, mockTranslator)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// 验证统计信息
	stats := processedDoc.ProcessingStats
	if stats.TotalNodes == 0 {
		t.Error("Should have processed some nodes")
	}
	if stats.TotalAttributes == 0 {
		t.Error("Should have processed some attributes")
	}

	t.Logf("Processing stats: %+v", stats)

	// 渲染结果
	var output strings.Builder
	err = processor.Render(context.Background(), processedDoc, &output)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	result := output.String()
	if !strings.Contains(result, "<!DOCTYPE html>") {
		t.Error("DOCTYPE should be preserved in output")
	}
	if !strings.Contains(result, "翻译:") {
		t.Error("Translation should be applied in output")
	}

	t.Logf("Rendered HTML length: %d", len(result))
}

// 辅助类型和函数

type MockPatternProtector struct {
	ProtectedPatterns []string
}

func (m *MockPatternProtector) ProtectPattern(text, pattern string) string {
	m.ProtectedPatterns = append(m.ProtectedPatterns, pattern)
	return text // 简化实现，实际应该应用保护
}

type MockTranslationError struct {
	message string
}

func (e *MockTranslationError) Error() string {
	return e.message
}

// BenchmarkHTMLProcessing 性能基准测试
func BenchmarkHTMLProcessing(b *testing.B) {
	logger := zap.NewNop()
	protector := NewHTMLProtector()
	options := DefaultEnhancedProcessorOptions()
	processor := NewHTMLEnhancedProcessor(logger, protector, options)

	testHTML := generateLargeHTML(100) // 生成包含100个段落的HTML

	mockTranslator := func(ctx context.Context, text string) (string, error) {
		return "TRANSLATED: " + text, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc, err := processor.Parse(context.Background(), strings.NewReader(testHTML))
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}

		_, err = processor.Process(context.Background(), doc, mockTranslator)
		if err != nil {
			b.Fatalf("Process failed: %v", err)
		}
	}
}

func generateLargeHTML(paragraphs int) string {
	var builder strings.Builder
	builder.WriteString(`<!DOCTYPE html><html><head><title>Test</title></head><body>`)

	for i := 0; i < paragraphs; i++ {
		builder.WriteString(`<p>This is paragraph `)
		builder.WriteString(string(rune('0' + (i % 10))))
		builder.WriteString(` with some content to translate.</p>`)
	}

	builder.WriteString(`</body></html>`)
	return builder.String()
}