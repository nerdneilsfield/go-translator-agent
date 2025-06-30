package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	mathjax "github.com/litao91/goldmark-mathjax"
	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func main() {
	// 检查命令行参数
	if len(os.Args) < 2 {
		fmt.Println("用法: go run markdown_ast_printer.go <markdown文件路径>")
		return
	}

	// 读取文件
	filePath := os.Args[1]
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("读取文件出错: %v\n", err)
		return
	}

	// 解析Markdown，添加扩展
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,            // GitHub Flavored Markdown
			extension.Typographer,    // 排版优化
			extension.TaskList,       // 任务列表
			extension.Table,          // 表格
			extension.Strikethrough,  // 删除线
			extension.DefinitionList, // 定义列表
			mathjax.MathJax,          // 数学公式
			meta.Meta,                // 元数据
			extension.Footnote,       // 脚注
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // 自动生成标题ID
			parser.WithAttribute(),     // 允许属性设置
		),
	)
	reader := text.NewReader(content)
	doc := md.Parser().Parse(reader)

	// 打印AST
	fmt.Println("=== Markdown AST 结构 ===")
	printAST(doc, reader, content, 0)
}

// printAST 递归打印AST节点
func printAST(n ast.Node, reader text.Reader, source []byte, level int) {
	indent := strings.Repeat("  ", level)

	// 获取节点类型名称
	nodeType := reflect.TypeOf(n).String()

	// 打印节点类型
	fmt.Printf("%s%s", indent, nodeType)

	// 打印节点是否是内联节点
	if n.Type() == ast.TypeInline {
		fmt.Printf(" (内联)")
	} else if n.Type() == ast.TypeBlock {
		fmt.Printf(" (块级)")
	}
	fmt.Println()

	// 显示文本内容
	switch node := n.(type) {
	case *ast.Text:
		content := string(node.Segment.Value(source))
		fmt.Printf("%s└─ 文本: %q\n", indent, content)
	case *ast.String:
		fmt.Printf("%s└─ 字符串: %q\n", indent, string(node.Value))
	case *ast.Link:
		fmt.Printf("%s└─ 链接: %s\n", indent, string(node.Destination))
		if len(node.Title) > 0 {
			fmt.Printf("%s└─ 链接标题: %s\n", indent, string(node.Title))
		}
	case *ast.Image:
		fmt.Printf("%s└─ 图片: %s\n", indent, string(node.Destination))
		if len(node.Title) > 0 {
			fmt.Printf("%s└─ 图片标题: %s\n", indent, string(node.Title))
		}
	case *ast.Heading:
		fmt.Printf("%s└─ 标题级别: %d\n", indent, node.Level)
		if len(node.Attributes()) > 0 {
			fmt.Printf("%s└─ 属性: %+v\n", indent, node.Attributes())
		}
	case *ast.FencedCodeBlock:
		if node.Info != nil {
			info := string(node.Info.Segment.Value(source))
			fmt.Printf("%s└─ 代码块语言: %s\n", indent, info)
		}
	case *ast.CodeBlock:
		fmt.Printf("%s└─ 普通代码块\n", indent)
	case *ast.CodeSpan:
		var contentBuilder strings.Builder
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if textNode, ok := child.(*ast.Text); ok {
				contentBuilder.Write(textNode.Segment.Value(source))
			}
		}
		content := contentBuilder.String()
		fmt.Printf("%s└─ 行内代码: %q\n", indent, content)
	case *ast.List:
		if node.IsOrdered() {
			fmt.Printf("%s└─ 有序列表(起始值: %d)\n", indent, node.Start)
		} else {
			fmt.Printf("%s└─ 无序列表\n", indent)
		}
	case *ast.ListItem:
		fmt.Printf("%s└─ 列表项\n", indent)
		if node.Offset != 0 {
			fmt.Printf("%s   └─ 偏移量: %d\n", indent, node.Offset)
		}
	case *ast.Paragraph:
		fmt.Printf("%s└─ 段落\n", indent)
	case *ast.Blockquote:
		fmt.Printf("%s└─ 引用块\n", indent)
	case *ast.HTMLBlock:
		content := string(node.Lines().Value(source))
		fmt.Printf("%s└─ HTML块: %q\n", indent, content)
	case *ast.RawHTML:
		content := string(node.Segments.Value(source))
		fmt.Printf("%s└─ 原始HTML: %q\n", indent, content)
	case *ast.ThematicBreak:
		fmt.Printf("%s└─ 水平分隔线\n", indent)
	case *ast.Emphasis:
		fmt.Printf("%s└─ 强调(级别: %d)\n", indent, node.Level)
	default:
		// 检查特殊节点类型
		if strings.Contains(nodeType, "TaskListItem") {
			fmt.Printf("%s└─ 任务复选框\n", indent)
			if item, ok := n.(interface{ IsChecked() bool }); ok {
				fmt.Printf("%s   └─ 已选中: %v\n", indent, item.IsChecked())
			}
		} else if strings.Contains(nodeType, "Math") {
			fmt.Printf("%s└─ 数学公式\n", indent)
			printMathContent(n, source, indent)
		} else if strings.Contains(nodeType, "Footnote") {
			fmt.Printf("%s└─ 脚注\n", indent)
		} else if strings.Contains(nodeType, "Definition") {
			fmt.Printf("%s└─ 定义列表项\n", indent)
		} else if strings.Contains(nodeType, "Table") {
			fmt.Printf("%s└─ 表格相关元素\n", indent)
		} else if strings.Contains(nodeType, "Strikethrough") {
			fmt.Printf("%s└─ 删除线\n", indent)
		}
	}

	// 显示行信息 (仅对块级节点)
	if n.Type() == ast.TypeBlock && n.Lines().Len() > 0 {
		startLine := n.Lines().At(0).Start
		endLine := n.Lines().At(n.Lines().Len() - 1).Stop
		fmt.Printf("%s└─ 行范围: %d-%d\n", indent, startLine, endLine)
	}

	// 递归处理子节点
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		printAST(child, reader, source, level+1)
	}
}

// printMathContent 安全地打印数学公式内容
func printMathContent(n ast.Node, source []byte, indent string) {
	// 首先尝试从子节点获取内容
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if textNode, ok := child.(*ast.Text); ok {
			content := string(textNode.Segment.Value(source))
			fmt.Printf("%s   └─ 公式内容: %q\n", indent, content)
			return
		}
	}

	// 如果没有找到子节点内容，尝试使用反射获取字段
	v := reflect.ValueOf(n)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() == reflect.Struct {
		for _, fieldName := range []string{"Literal", "Content", "Value"} {
			field := v.FieldByName(fieldName)
			if field.IsValid() && field.CanInterface() {
				value := field.Interface()
				if strValue, ok := value.(string); ok {
					fmt.Printf("%s   └─ 公式内容(字段): %q\n", indent, strValue)
					return
				} else if byteValue, ok := value.([]byte); ok {
					fmt.Printf("%s   └─ 公式内容(字段): %q\n", indent, string(byteValue))
					return
				}
			}
		}
	}
}
