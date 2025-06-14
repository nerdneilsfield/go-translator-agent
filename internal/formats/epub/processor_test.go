package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/nerdneilsfield/go-translator-agent/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestEPUB 创建测试用的 EPUB 文件
func createTestEPUB(t *testing.T) []byte {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// 添加 mimetype 文件
	mimetype, err := zipWriter.Create("mimetype")
	require.NoError(t, err)
	_, err = mimetype.Write([]byte("application/epub+zip"))
	require.NoError(t, err)

	// 添加 META-INF/container.xml
	container, err := zipWriter.Create("META-INF/container.xml")
	require.NoError(t, err)
	containerXML := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
    <rootfiles>
        <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
    </rootfiles>
</container>`
	_, err = container.Write([]byte(containerXML))
	require.NoError(t, err)

	// 添加 content.opf
	opf, err := zipWriter.Create("OEBPS/content.opf")
	require.NoError(t, err)
	opfContent := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <dc:title>Test Book</dc:title>
        <dc:creator>Test Author</dc:creator>
        <dc:language>en</dc:language>
    </metadata>
    <manifest>
        <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
        <item id="chapter2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
        <item id="style" href="style.css" media-type="text/css"/>
    </manifest>
    <spine>
        <itemref idref="chapter1"/>
        <itemref idref="chapter2"/>
    </spine>
</package>`
	_, err = opf.Write([]byte(opfContent))
	require.NoError(t, err)

	// 添加 chapter1.xhtml
	chapter1, err := zipWriter.Create("OEBPS/chapter1.xhtml")
	require.NoError(t, err)
	chapter1Content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
    <title>Chapter 1</title>
    <link rel="stylesheet" type="text/css" href="style.css"/>
</head>
<body>
    <h1>Chapter One</h1>
    <p>This is the first paragraph of chapter one.</p>
    <p>This is the <strong>second paragraph</strong> with some <em>emphasis</em>.</p>
</body>
</html>`
	_, err = chapter1.Write([]byte(chapter1Content))
	require.NoError(t, err)

	// 添加 chapter2.xhtml
	chapter2, err := zipWriter.Create("OEBPS/chapter2.xhtml")
	require.NoError(t, err)
	chapter2Content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
    <title>Chapter 2</title>
    <link rel="stylesheet" type="text/css" href="style.css"/>
</head>
<body>
    <h1>Chapter Two</h1>
    <p>This is the content of chapter two.</p>
    <ul>
        <li>First item</li>
        <li>Second item</li>
    </ul>
</body>
</html>`
	_, err = chapter2.Write([]byte(chapter2Content))
	require.NoError(t, err)

	// 添加 style.css
	style, err := zipWriter.Create("OEBPS/style.css")
	require.NoError(t, err)
	styleContent := `body {
    font-family: Georgia, serif;
    margin: 1em;
}
h1 {
    color: #333;
    font-size: 2em;
}`
	_, err = style.Write([]byte(styleContent))
	require.NoError(t, err)

	err = zipWriter.Close()
	require.NoError(t, err)

	return buf.Bytes()
}

func TestEPUBProcessor(t *testing.T) {
	// 创建测试 EPUB
	epubData := createTestEPUB(t)
	ctx := context.Background()

	// 测试两种模式
	modes := []struct {
		name string
		mode ProcessingMode
	}{
		{"MarkdownMode", ModeMarkdown},
		{"NativeMode", ModeNative},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			// 创建指定模式的处理器
			processor, err := ProcessorWithMode(tc.mode, document.ProcessorOptions{
				ChunkSize:    1000,
				ChunkOverlap: 50,
			})
			require.NoError(t, err)
			require.NotNil(t, processor)

			// 测试解析
			t.Run("Parse", func(t *testing.T) {
				reader := bytes.NewReader(epubData)
				doc, err := processor.Parse(ctx, reader)
				require.NoError(t, err)
				require.NotNil(t, doc)

				// 验证文档格式
				assert.Equal(t, document.FormatEPUB, doc.Format)

				// 验证元数据
				assert.Equal(t, "Test Book", doc.Metadata.Title)
				assert.Equal(t, "Test Author", doc.Metadata.Author)
				assert.Equal(t, "en", doc.Metadata.Language)

				// 验证提取了内容
				assert.NotEmpty(t, doc.Blocks)

				// 验证提取了关键文本
				allContent := strings.Join(getAllBlockContents(doc.Blocks), " ")
				assert.Contains(t, allContent, "Chapter One")
				assert.Contains(t, allContent, "Chapter Two")
				assert.Contains(t, allContent, "first paragraph")

				// 验证 EPUB 文件结构被保存
				epubFiles, ok := doc.Metadata.CustomFields["epub_files"].(map[string][]byte)
				require.True(t, ok)
				assert.NotEmpty(t, epubFiles)
				assert.Contains(t, epubFiles, "OEBPS/chapter1.xhtml")
				assert.Contains(t, epubFiles, "OEBPS/chapter2.xhtml")
				assert.Contains(t, epubFiles, "OEBPS/style.css")
			})

			// 测试处理（翻译）
			t.Run("Process", func(t *testing.T) {
				reader := bytes.NewReader(epubData)
				doc, err := processor.Parse(ctx, reader)
				require.NoError(t, err)

				// 模拟翻译函数
				translateFunc := func(ctx context.Context, text string) (string, error) {
					// 简单的替换翻译
					translated := strings.ReplaceAll(text, "Chapter", "章节")
					translated = strings.ReplaceAll(translated, "paragraph", "段落")
					translated = strings.ReplaceAll(translated, "item", "项目")
					return translated, nil
				}

				processedDoc, err := processor.Process(ctx, doc, translateFunc)
				require.NoError(t, err)
				require.NotNil(t, processedDoc)

				// 验证翻译生效
				epubFiles, ok := processedDoc.Metadata.CustomFields["epub_files"].(map[string][]byte)
				require.True(t, ok)

				// 检查 chapter1.xhtml 是否被翻译
				chapter1Content := string(epubFiles["OEBPS/chapter1.xhtml"])
				assert.Contains(t, chapter1Content, "章节")
				assert.Contains(t, chapter1Content, "段落")

				// 检查 chapter2.xhtml 是否被翻译
				chapter2Content := string(epubFiles["OEBPS/chapter2.xhtml"])
				assert.Contains(t, chapter2Content, "章节")
				assert.Contains(t, chapter2Content, "项目")
			})

			// 测试渲染
			t.Run("Render", func(t *testing.T) {
				reader := bytes.NewReader(epubData)
				doc, err := processor.Parse(ctx, reader)
				require.NoError(t, err)

				// 翻译
				translateFunc := func(ctx context.Context, text string) (string, error) {
					return "[TRANSLATED] " + text, nil
				}
				processedDoc, err := processor.Process(ctx, doc, translateFunc)
				require.NoError(t, err)

				// 渲染
				var output bytes.Buffer
				err = processor.Render(ctx, processedDoc, &output)
				require.NoError(t, err)

				// 验证输出是有效的 ZIP 文件
				outputData := output.Bytes()
				assert.NotEmpty(t, outputData)

				// 验证可以作为 ZIP 文件打开
				zipReader, err := zip.NewReader(bytes.NewReader(outputData), int64(len(outputData)))
				require.NoError(t, err)

				// 验证文件结构保持完整
				fileNames := make([]string, 0)
				for _, file := range zipReader.File {
					fileNames = append(fileNames, file.Name)
				}
				assert.Contains(t, fileNames, "mimetype")
				assert.Contains(t, fileNames, "OEBPS/content.opf")
				assert.Contains(t, fileNames, "OEBPS/chapter1.xhtml")
				assert.Contains(t, fileNames, "OEBPS/chapter2.xhtml")
				assert.Contains(t, fileNames, "OEBPS/style.css")

				// 验证翻译的内容在输出中
				for _, file := range zipReader.File {
					if strings.HasSuffix(file.Name, ".xhtml") {
						reader, err := file.Open()
						require.NoError(t, err)
						defer reader.Close()

						data, err := io.ReadAll(reader)
						require.NoError(t, err)

						content := string(data)
						assert.Contains(t, content, "[TRANSLATED]")
					}
				}
			})
		})
	}
}

// getAllBlockContents 获取所有块的内容
func getAllBlockContents(blocks []document.Block) []string {
	var contents []string
	for _, block := range blocks {
		contents = append(contents, block.GetContent())
	}
	return contents
}
