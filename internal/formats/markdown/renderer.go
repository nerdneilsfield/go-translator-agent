package markdown

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
)

// Renderer Markdown 渲染器
type Renderer struct{}

// NewRenderer 创建 Markdown 渲染器
func NewRenderer() *Renderer {
	return &Renderer{}
}

// Render 将文档渲染为 Markdown 格式
func (r *Renderer) Render(ctx context.Context, doc *document.Document, output io.Writer) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}

	var result strings.Builder

	// 渲染每个块
	for i, block := range doc.Blocks {
		blockContent := r.renderBlock(block)
		result.WriteString(blockContent)
		
		// 在块之间添加适当的空行
		if i < len(doc.Blocks)-1 {
			nextBlock := doc.Blocks[i+1]
			spacing := r.getBlockSpacing(block, nextBlock)
			result.WriteString(spacing)
		}
	}

	// 写入输出
	_, err := output.Write([]byte(result.String()))
	return err
}

// CanRender 检查是否能渲染该格式
func (r *Renderer) CanRender(format document.Format) bool {
	return format == document.FormatMarkdown
}

// renderBlock 渲染单个块
func (r *Renderer) renderBlock(block document.Block) string {
	if block == nil {
		return ""
	}

	content := block.GetContent()
	
	switch block.GetType() {
	case document.BlockTypeHeading:
		// 标题已经包含了 # 符号
		return content
		
	case document.BlockTypeCode:
		// 代码块已经包含了 ``` 标记
		return content
		
	case document.BlockTypeMath:
		// 数学块已经包含了 $$ 标记
		return content
		
	case document.BlockTypeList:
		// 列表已经包含了正确的格式
		return content
		
	case document.BlockTypeTable:
		// 表格已经包含了正确的格式
		return content
		
	case document.BlockTypeQuote:
		// 引用已经包含了 > 标记
		return content
		
	case document.BlockTypeImage:
		// 图片已经包含了 ![](url) 格式
		return content
		
	case document.BlockTypeParagraph:
		// 段落直接返回内容
		return content
		
	default:
		// 其他类型直接返回内容
		return content
	}
}

// getBlockSpacing 获取块之间的间距
func (r *Renderer) getBlockSpacing(current, next document.Block) string {
	// 默认使用双换行分隔段落
	defaultSpacing := "\n\n"
	
	if current == nil || next == nil {
		return defaultSpacing
	}
	
	currentType := current.GetType()
	nextType := next.GetType()
	
	// 列表项之间不需要额外空行
	if currentType == document.BlockTypeList && nextType == document.BlockTypeList {
		// 检查是否是同一个列表
		nextContent := next.GetContent()
		
		// 如果下一个块是缩进的，说明是同一个列表的子项
		if strings.HasPrefix(nextContent, "  ") || strings.HasPrefix(nextContent, "\t") {
			return "\n"
		}
	}
	
	// 代码块内部已经包含了换行
	if currentType == document.BlockTypeCode {
		return "\n\n"
	}
	
	// 标题前后通常需要额外的空行
	if nextType == document.BlockTypeHeading {
		return "\n\n"
	}
	
	return defaultSpacing
}