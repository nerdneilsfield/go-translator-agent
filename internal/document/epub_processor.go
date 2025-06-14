package document

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"
)

// EPUBProcessor EPUB文档处理器，使用 NodeInfo 系统
type EPUBProcessor struct {
	opts           ProcessorOptions
	logger         *zap.Logger
	nodeTranslator *NodeInfoTranslator
	mode           HTMLProcessingMode
	htmlProcessor  *HTMLProcessor
}

// NewEPUBProcessor 创建EPUB处理器
func NewEPUBProcessor(opts ProcessorOptions, logger *zap.Logger, mode HTMLProcessingMode) (*EPUBProcessor, error) {
	// 设置默认值
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = 2000
	}
	if opts.ChunkOverlap < 0 {
		opts.ChunkOverlap = 100
	}

	// 创建节点翻译器
	contextDistance := 2
	maxRetries := 3
	nodeTranslator := NewNodeInfoTranslator(opts.ChunkSize, contextDistance, maxRetries)

	// 创建内部HTML处理器
	htmlProcessor, err := NewHTMLProcessor(opts, logger, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTML processor: %w", err)
	}

	return &EPUBProcessor{
		opts:           opts,
		logger:         logger,
		nodeTranslator: nodeTranslator,
		mode:           mode,
		htmlProcessor:  htmlProcessor,
	}, nil
}

// Parse 解析EPUB输入
func (p *EPUBProcessor) Parse(ctx context.Context, input io.Reader) (*Document, error) {
	// 读取所有内容到内存
	content, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	// 创建zip reader
	zipReader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("failed to open EPUB as zip: %w", err)
	}

	// 创建文档
	doc := &Document{
		ID:     fmt.Sprintf("epub-%d", time.Now().Unix()),
		Format: FormatEPUB,
		Metadata: DocumentMetadata{
			CreatedAt: time.Now(),
			CustomFields: map[string]interface{}{
				"epubData":  content,
				"zipReader": zipReader,
				"mode":      p.mode,
			},
		},
		Blocks:    []Block{},
		Resources: make(map[string]Resource),
	}

	// 查找content.opf文件
	opfPath, err := p.findOPFPath(zipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to find OPF file: %w", err)
	}

	// 解析OPF文件
	opfFile, err := p.readZipFile(zipReader, opfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read OPF file: %w", err)
	}

	// 解析manifest和spine
	manifest, spine, err := p.parseOPF(opfFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OPF: %w", err)
	}

	// 保存manifest和spine
	doc.Metadata.CustomFields["manifest"] = manifest
	doc.Metadata.CustomFields["spine"] = spine
	doc.Metadata.CustomFields["opfPath"] = opfPath

	// 处理每个HTML文件
	nodeIDOffset := 1
	for i, itemRef := range spine {
		item, exists := manifest[itemRef.IDRef]
		if !exists {
			p.logger.Warn("spine item not found in manifest", zap.String("idref", itemRef.IDRef))
			continue
		}

		// 读取HTML文件
		htmlPath := path.Join(path.Dir(opfPath), item.Href)
		htmlContent, err := p.readZipFile(zipReader, htmlPath)
		if err != nil {
			p.logger.Warn("failed to read HTML file",
				zap.String("path", htmlPath),
				zap.Error(err))
			continue
		}

		// 创建HTML块
		block := &BaseBlock{
			Type:         BlockTypeHTML,
			Content:      string(htmlContent),
			Translatable: true,
			Metadata: BlockMetadata{
				Attributes: map[string]interface{}{
					"htmlPath":   htmlPath,
					"spineIndex": i,
					"manifestID": itemRef.IDRef,
				},
			},
		}
		doc.Blocks = append(doc.Blocks, block)

		// 使用内部HTML处理器解析HTML内容
		_, err = p.htmlProcessor.Parse(ctx, bytes.NewReader(htmlContent))
		if err != nil {
			p.logger.Warn("failed to parse HTML in EPUB",
				zap.String("path", htmlPath),
				zap.Error(err))
			continue
		}

		// 将HTML的节点添加到我们的集合中，调整ID避免冲突
		htmlNodes := p.htmlProcessor.nodeTranslator.collection.GetAll()
		for _, node := range htmlNodes {
			// 创建新节点，调整ID和路径
			epubNode := &NodeInfo{
				ID:             nodeIDOffset + node.ID - 1,
				BlockID:        fmt.Sprintf("epub-%d-%s", i, node.BlockID),
				OriginalText:   node.OriginalText,
				TranslatedText: node.TranslatedText,
				Status:         node.Status,
				Path:           fmt.Sprintf("%s%s", htmlPath, node.Path),
				ContextBefore:  node.ContextBefore,
				ContextAfter:   node.ContextAfter,
				Metadata:       node.Metadata,
				Error:          node.Error,
				RetryCount:     node.RetryCount,
			}

			// 添加EPUB特定的元数据
			if epubNode.Metadata == nil {
				epubNode.Metadata = make(map[string]interface{})
			}
			epubNode.Metadata["epubHtmlPath"] = htmlPath
			epubNode.Metadata["epubSpineIndex"] = i

			p.nodeTranslator.collection.Add(epubNode)
		}

		nodeIDOffset += len(htmlNodes)
	}

	// 收集非HTML资源（图片、CSS等）
	for id, item := range manifest {
		if !p.isHTMLFile(item.Href) {
			resourcePath := path.Join(path.Dir(opfPath), item.Href)
			resourceData, err := p.readZipFile(zipReader, resourcePath)
			if err != nil {
				p.logger.Warn("failed to read resource",
					zap.String("path", resourcePath),
					zap.Error(err))
				continue
			}

			doc.Resources[id] = Resource{
				ID:          id,
				Type:        p.getResourceType(item.MediaType),
				ContentType: item.MediaType,
				Data:        resourceData,
				URL:         resourcePath,
			}
		}
	}

	return doc, nil
}

// Process 处理文档
func (p *EPUBProcessor) Process(ctx context.Context, doc *Document, translator TranslateFunc) (*Document, error) {
	startTime := time.Now()
	stats := ProcessingStatistics{}

	// 获取所有待翻译的节点
	allNodes := p.nodeTranslator.collection.GetAll()
	stats.TotalBlocks = len(allNodes)

	// 使用HTML处理器的翻译逻辑
	err := p.htmlProcessor.translateNodes(ctx, allNodes, translator, &stats)
	if err != nil {
		return nil, fmt.Errorf("initial translation failed: %w", err)
	}

	// 重试失败的节点
	for retry := 0; retry < 3; retry++ {
		retryGroups, err := p.nodeTranslator.retryManager.PrepareRetryGroups()
		if err != nil {
			p.logger.Warn("failed to prepare retry groups", zap.Error(err))
			break
		}

		if len(retryGroups) == 0 {
			break
		}

		p.logger.Info("retrying failed nodes",
			zap.Int("retry", retry+1),
			zap.Int("groups", len(retryGroups)))

		// 翻译重试组
		for _, group := range retryGroups {
			markedText := p.htmlProcessor.generateMarkedText(group.Nodes)
			translatedText, err := translator(ctx, markedText)
			if err != nil {
				for _, node := range group.Nodes {
					if node.Status == NodeStatusFailed {
						p.nodeTranslator.retryManager.MarkRetryCompleted(node.ID, false, "", err)
					}
				}
				continue
			}

			// 解析翻译结果
			p.htmlProcessor.parseMarkedText(translatedText, group.Nodes)

			// 标记重试完成
			for _, node := range group.Nodes {
				if node.Status == NodeStatusSuccess {
					p.nodeTranslator.retryManager.MarkRetryCompleted(node.ID, true, node.TranslatedText, nil)
				}
			}
		}

		// 重置已处理节点
		p.nodeTranslator.retryManager.ResetProcessedNodes()
	}

	// 统计翻译结果
	translatedNodes := p.nodeTranslator.collection.GetByStatus(NodeStatusSuccess)
	stats.TranslatedBlocks = len(translatedNodes)
	stats.SkippedBlocks = stats.TotalBlocks - stats.TranslatedBlocks
	stats.ProcessingTime = time.Since(startTime)

	p.logger.Info("EPUB processing completed",
		zap.Int("totalBlocks", stats.TotalBlocks),
		zap.Int("translatedBlocks", stats.TranslatedBlocks),
		zap.Int("skippedBlocks", stats.SkippedBlocks),
		zap.Duration("processingTime", stats.ProcessingTime))

	return doc, nil
}

// Render 渲染文档
func (p *EPUBProcessor) Render(ctx context.Context, doc *Document, output io.Writer) error {
	// 获取原始EPUB数据
	epubData, ok := doc.Metadata.CustomFields["epubData"].([]byte)
	if !ok {
		return fmt.Errorf("no EPUB data found")
	}

	// 创建新的zip writer
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// 读取原始zip
	zipReader, err := zip.NewReader(bytes.NewReader(epubData), int64(len(epubData)))
	if err != nil {
		return fmt.Errorf("failed to read original EPUB: %w", err)
	}

	// 获取manifest和spine
	_, _ = doc.Metadata.CustomFields["manifest"].(map[string]ManifestItem)
	_, _ = doc.Metadata.CustomFields["spine"].([]SpineItemRef)
	_, _ = doc.Metadata.CustomFields["opfPath"].(string)

	// 创建HTML路径到翻译内容的映射
	translatedHTML := make(map[string]string)

	// 处理每个HTML块
	for _, block := range doc.Blocks {
		if block.GetType() != BlockTypeHTML {
			continue
		}

		htmlPath, _ := block.GetMetadata().Attributes["htmlPath"].(string)
		spineIndex, _ := block.GetMetadata().Attributes["spineIndex"].(int)

		// 收集这个HTML文件的所有翻译节点
		var htmlNodes []*NodeInfo
		allNodes := p.nodeTranslator.collection.GetAll()
		for _, node := range allNodes {
			if epubSpineIndex, ok := node.Metadata["epubSpineIndex"].(int); ok && epubSpineIndex == spineIndex {
				htmlNodes = append(htmlNodes, node)
			}
		}

		// 如果有翻译的节点，重新渲染HTML
		if len(htmlNodes) > 0 {
			originalHTML := block.GetContent()

			// 创建临时的HTML文档用于渲染
			tempDoc := &Document{
				Format: FormatHTML,
				Metadata: DocumentMetadata{
					CustomFields: map[string]interface{}{
						"originalHTML": originalHTML,
						"mode":         p.mode,
					},
				},
			}

			// 如果是Markdown模式
			if p.mode == HTMLModeMarkdown {
				// 找到对应的Markdown节点并设置翻译内容
				for _, node := range htmlNodes {
					if node.Metadata["type"] == "markdown" && node.IsTranslated() {
						tempDoc.Blocks = append(tempDoc.Blocks, &BaseBlock{
							Type:         BlockTypeCustom,
							Content:      node.TranslatedText,
							Translatable: true,
							Metadata: BlockMetadata{
								Attributes: map[string]interface{}{
									"format": "markdown",
								},
							},
						})
						break
					}
				}
			}

			// 渲染HTML
			var htmlBuf bytes.Buffer
			if err := p.htmlProcessor.Render(ctx, tempDoc, &htmlBuf); err != nil {
				p.logger.Warn("failed to render HTML",
					zap.String("path", htmlPath),
					zap.Error(err))
				continue
			}

			translatedHTML[htmlPath] = htmlBuf.String()
		}
	}

	// 复制所有文件，替换翻译后的HTML
	for _, file := range zipReader.File {
		// 检查是否是需要替换的HTML文件
		if translatedContent, exists := translatedHTML[file.Name]; exists {
			// 创建新文件
			writer, err := zipWriter.Create(file.Name)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", file.Name, err)
			}

			// 写入翻译后的内容
			if _, err := writer.Write([]byte(translatedContent)); err != nil {
				return fmt.Errorf("failed to write translated content: %w", err)
			}
		} else {
			// 复制原始文件
			reader, err := file.Open()
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", file.Name, err)
			}

			writer, err := zipWriter.Create(file.Name)
			if err != nil {
				reader.Close()
				return fmt.Errorf("failed to create file %s: %w", file.Name, err)
			}

			if _, err := io.Copy(writer, reader); err != nil {
				reader.Close()
				return fmt.Errorf("failed to copy file %s: %w", file.Name, err)
			}
			reader.Close()
		}
	}

	// 关闭zip writer
	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close zip: %w", err)
	}

	// 写入输出
	_, err = output.Write(buf.Bytes())
	return err
}

// GetFormat 返回支持的格式
func (p *EPUBProcessor) GetFormat() Format {
	return FormatEPUB
}

// EPUB相关结构体
type Package struct {
	XMLName  xml.Name `xml:"package"`
	Manifest Manifest `xml:"manifest"`
	Spine    Spine    `xml:"spine"`
	Metadata Metadata `xml:"metadata"`
}

type Manifest struct {
	Items []ManifestItem `xml:"item"`
}

type ManifestItem struct {
	ID        string `xml:"id,attr"`
	Href      string `xml:"href,attr"`
	MediaType string `xml:"media-type,attr"`
}

type Spine struct {
	ItemRefs []SpineItemRef `xml:"itemref"`
}

type SpineItemRef struct {
	IDRef string `xml:"idref,attr"`
}

type Metadata struct {
	Title    string `xml:"title"`
	Language string `xml:"language"`
	Creator  string `xml:"creator"`
}

// 辅助方法

func (p *EPUBProcessor) findOPFPath(zipReader *zip.Reader) (string, error) {
	// 首先查找META-INF/container.xml
	for _, file := range zipReader.File {
		if file.Name == "META-INF/container.xml" {
			reader, err := file.Open()
			if err != nil {
				return "", err
			}
			defer reader.Close()

			// 解析container.xml
			var container struct {
				Rootfiles struct {
					Rootfile struct {
						FullPath string `xml:"full-path,attr"`
					} `xml:"rootfile"`
				} `xml:"rootfiles"`
			}

			decoder := xml.NewDecoder(reader)
			if err := decoder.Decode(&container); err != nil {
				return "", err
			}

			return container.Rootfiles.Rootfile.FullPath, nil
		}
	}

	// 如果没有container.xml，查找*.opf文件
	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, ".opf") {
			return file.Name, nil
		}
	}

	return "", fmt.Errorf("OPF file not found")
}

func (p *EPUBProcessor) readZipFile(zipReader *zip.Reader, path string) ([]byte, error) {
	for _, file := range zipReader.File {
		if file.Name == path {
			reader, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer reader.Close()

			return io.ReadAll(reader)
		}
	}
	return nil, fmt.Errorf("file not found: %s", path)
}

func (p *EPUBProcessor) parseOPF(opfContent []byte) (map[string]ManifestItem, []SpineItemRef, error) {
	var pkg Package
	if err := xml.Unmarshal(opfContent, &pkg); err != nil {
		return nil, nil, err
	}

	// 创建manifest映射
	manifest := make(map[string]ManifestItem)
	for _, item := range pkg.Manifest.Items {
		manifest[item.ID] = item
	}

	return manifest, pkg.Spine.ItemRefs, nil
}

func (p *EPUBProcessor) isHTMLFile(href string) bool {
	ext := strings.ToLower(path.Ext(href))
	return ext == ".html" || ext == ".xhtml" || ext == ".htm"
}

func (p *EPUBProcessor) getResourceType(mediaType string) ResourceType {
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		return ResourceTypeImage
	case mediaType == "text/css":
		return ResourceTypeStylesheet
	case strings.HasPrefix(mediaType, "font/"):
		return ResourceTypeFont
	default:
		return ResourceTypeOther
	}
}
