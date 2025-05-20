package translator_tests

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/pkg/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// 创建测试用的EPUB文件
func createTestEPUB(t *testing.T, dir string) string {
	epubPath := filepath.Join(dir, "test.epub")

	// 创建一个简单的EPUB文件（实际上是一个ZIP文件）
	zipFile, err := os.Create(epubPath)
	assert.NoError(t, err)
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 添加mimetype文件（不压缩）
	mimetypeWriter, err := zipWriter.CreateHeader(&zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store, // 不压缩
	})
	assert.NoError(t, err)
	_, err = mimetypeWriter.Write([]byte("application/epub+zip"))
	assert.NoError(t, err)

	// 添加META-INF/container.xml
	containerWriter, err := zipWriter.Create("META-INF/container.xml")
	assert.NoError(t, err)
	_, err = containerWriter.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))
	assert.NoError(t, err)

	// 添加OEBPS/content.opf
	contentWriter, err := zipWriter.Create("OEBPS/content.opf")
	assert.NoError(t, err)
	_, err = contentWriter.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="BookID" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Test Book</dc:title>
    <dc:language>en</dc:language>
    <dc:identifier id="BookID">test-book-id</dc:identifier>
  </metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="chapter2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx">
    <itemref idref="chapter1"/>
    <itemref idref="chapter2"/>
  </spine>
</package>`))
	assert.NoError(t, err)

	// 添加OEBPS/toc.ncx
	tocWriter, err := zipWriter.Create("OEBPS/toc.ncx")
	assert.NoError(t, err)
	_, err = tocWriter.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <head>
    <meta name="dtb:uid" content="test-book-id"/>
    <meta name="dtb:depth" content="1"/>
    <meta name="dtb:totalPageCount" content="0"/>
    <meta name="dtb:maxPageNumber" content="0"/>
  </head>
  <docTitle>
    <text>Test Book</text>
  </docTitle>
  <navMap>
    <navPoint id="navPoint-1" playOrder="1">
      <navLabel>
        <text>Chapter 1</text>
      </navLabel>
      <content src="chapter1.xhtml"/>
    </navPoint>
    <navPoint id="navPoint-2" playOrder="2">
      <navLabel>
        <text>Chapter 2</text>
      </navLabel>
      <content src="chapter2.xhtml"/>
    </navPoint>
  </navMap>
</ncx>`))
	assert.NoError(t, err)

	// 添加OEBPS/chapter1.xhtml
	chapter1Writer, err := zipWriter.Create("OEBPS/chapter1.xhtml")
	assert.NoError(t, err)
	_, err = chapter1Writer.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
  <title>Chapter 1</title>
</head>
<body>
  <h1>Chapter 1: Introduction</h1>
  <p>This is the first paragraph of chapter 1.</p>
  <p>This is the second paragraph with some <em>emphasized text</em>.</p>
  <p>This is the third paragraph with a <a href="#note1">link</a>.</p>
</body>
</html>`))
	assert.NoError(t, err)

	// 添加OEBPS/chapter2.xhtml
	chapter2Writer, err := zipWriter.Create("OEBPS/chapter2.xhtml")
	assert.NoError(t, err)
	_, err = chapter2Writer.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
  <title>Chapter 2</title>
</head>
<body>
  <h1>Chapter 2: Content</h1>
  <p>This is the first paragraph of chapter 2.</p>
  <p>This is the second paragraph with some <strong>bold text</strong>.</p>
  <p>This is the last paragraph of the chapter.</p>
</body>
</html>`))
	assert.NoError(t, err)

	return epubPath
}

// 从EPUB文件中提取指定文件的内容
func extractFileFromEPUB(t *testing.T, epubPath, filePath string) string {
	reader, err := zip.OpenReader(epubPath)
	assert.NoError(t, err)
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name == filePath {
			fileReader, err := file.Open()
			assert.NoError(t, err)
			defer fileReader.Close()

			content, err := io.ReadAll(fileReader)
			assert.NoError(t, err)

			return string(content)
		}
	}

	t.Fatalf("文件 %s 在EPUB中不存在", filePath)
	return ""
}

// 测试EPUB格式的翻译
func TestEPUBTranslation(t *testing.T) {
	// 创建模拟服务器
	server := NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置默认响应
	server.SetDefaultResponse("这是翻译后的文本")

	// 创建logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()

	// 创建配置
	cfg := createTestConfig()
	// 设置模型配置
	cfg.ModelConfigs["test-model"] = config.ModelConfig{
		Name:            "test-model",
		APIType:         "openai",
		BaseURL:         server.URL,
		Key:             "sk-test",
		MaxInputTokens:  8000,
		MaxOutputTokens: 2000,
	}

	// 创建日志
	_ = zapLogger

	// 创建模拟的缓存
	mockCache := new(MockCache)
	mockCache.On("Get", mock.Anything).Return("", false)
	mockCache.On("Set", mock.Anything, mock.Anything).Return(nil)

	// 创建翻译器
	_, err := translator.New(cfg, translator.WithCache(mockCache))
	assert.NoError(t, err)

	// 创建临时目录
	tempDir := t.TempDir()

	// 创建测试EPUB文件
	_ = createTestEPUB(t, tempDir)
	_ = filepath.Join(tempDir, "test_translated.epub")

	// 创建模拟翻译器
	mockTrans := NewMockTranslator(cfg, zapLogger)
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)

	// 执行翻译
	epubPath := createTestEPUB(t, tempDir)
	outputPath := filepath.Join(tempDir, "test_translated.epub")
	err = mockTrans.TranslateFile(epubPath, outputPath)
	assert.NoError(t, err)

	// 验证输出文件存在
	_, err2 := os.Stat(outputPath)
	assert.NoError(t, err2)

	// 提取并验证翻译后的章节内容
	chapter1Content := extractFileFromEPUB(t, outputPath, "OEBPS/chapter1.xhtml")
	chapter2Content := extractFileFromEPUB(t, outputPath, "OEBPS/chapter2.xhtml")

	// 验证章节内容已被翻译
	assert.Contains(t, chapter1Content, "这是翻译后的文本")
	assert.Contains(t, chapter2Content, "这是翻译后的文本")
}

// 测试EPUB章节末尾翻译问题
func TestEPUBChapterEndTranslation(t *testing.T) {
	// 创建模拟服务器
	server := NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置特定响应
	server.AddResponse("This is the last paragraph of the chapter.", "这是本章的最后一段。")

	// 创建logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()

	// 创建配置
	cfg := createTestConfig()
	// 设置模型配置
	cfg.ModelConfigs["test-model"] = config.ModelConfig{
		Name:            "test-model",
		APIType:         "openai",
		BaseURL:         server.URL,
		Key:             "sk-test",
		MaxInputTokens:  8000,
		MaxOutputTokens: 2000,
	}

	// 创建日志
	_ = zapLogger

	// 创建模拟的缓存
	mockCache := new(MockCache)
	mockCache.On("Get", mock.Anything).Return("", false)
	mockCache.On("Set", mock.Anything, mock.Anything).Return(nil)

	// 创建翻译器
	_, err := translator.New(cfg, translator.WithCache(mockCache))
	assert.NoError(t, err)

	// 创建临时目录
	tempDir := t.TempDir()

	// 创建测试EPUB文件
	_ = createTestEPUB(t, tempDir)
	_ = filepath.Join(tempDir, "test_end_translated.epub")

	// 创建模拟翻译器
	mockTrans := NewMockTranslator(cfg, zapLogger)
	mockTrans.SetPredefinedResult("This is the last paragraph of the book.", "这是本书的最后一段。")
	mockTrans.On("Translate", mock.Anything, mock.Anything).Return("这是翻译后的文本", nil)

	// 执行翻译
	epubPath := createTestEPUB(t, tempDir)
	outputPath := filepath.Join(tempDir, "test_end_translated.epub")
	err3 := mockTrans.TranslateFile(epubPath, outputPath)
	assert.NoError(t, err3)

	// 验证输出文件存在
	_, err2 := os.Stat(outputPath)
	assert.NoError(t, err2)

	// 提取并验证翻译后的章节内容
	chapter2Content := extractFileFromEPUB(t, outputPath, "OEBPS/chapter2.xhtml")

	// 验证章节末尾内容已被正确翻译
	assert.Contains(t, chapter2Content, "这是本书的最后一段")
}
