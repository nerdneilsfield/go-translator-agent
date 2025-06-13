package text

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/nerdneilsfield/go-translator-agent/pkg/document"
)

// Renderer 纯文本渲染器
type Renderer struct{}

// NewRenderer 创建纯文本渲染器
func NewRenderer() *Renderer {
	return &Renderer{}
}

// Render 将文档渲染为纯文本格式
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
			spacing := r.getBlockSpacing(block, doc.Blocks[i+1])
			result.WriteString(spacing)
		}
	}

	// 写入输出
	_, err := output.Write([]byte(result.String()))
	return err
}

// CanRender 检查是否能渲染该格式
func (r *Renderer) CanRender(format document.Format) bool {
	return format == document.FormatText
}

// renderBlock 渲染单个块
func (r *Renderer) renderBlock(block document.Block) string {
	if block == nil {
		return ""
	}

	content := block.GetContent()
	
	switch block.GetType() {
	case document.BlockTypeHeading:
		// 对于纯文本，标题可以保持原样或添加一些格式
		// 这里我们选择保持原样
		return content
		
	case document.BlockTypeList:
		// 列表项保持原样
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
	if current == nil || next == nil {
		return "\n\n"
	}
	
	currentType := current.GetType()
	nextType := next.GetType()
	
	// 列表项之间可能只需要单个换行
	if currentType == document.BlockTypeList && nextType == document.BlockTypeList {
		// 检查是否是连续的列表项
		return "\n"
	}
	
	// 标题前后通常需要额外的空行
	if currentType == document.BlockTypeHeading || nextType == document.BlockTypeHeading {
		return "\n\n"
	}
	
	// 默认段落之间使用双换行
	return "\n\n"
}