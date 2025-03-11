# 翻译工具 (Go-Translator-Agent)

一个高质量、灵活的多语言翻译系统，采用三步翻译流程确保翻译质量。

## 功能特点

- **三步翻译流程**：初始翻译、反思、改进
- **多模型支持**：可配置不同阶段使用不同的语言模型
- **多格式支持**：Markdown、纯文本，计划支持EPUB、LaTeX
- **格式保留**：保持原始文档的格式、结构和特殊元素
- **灵活缓存**：避免重复翻译，提高效率和一致性
- **命令行界面**：便于批处理和脚本集成
- **智能分块**：自动按段落和句子分割文本，支持并行翻译
- **自动保护**：自动保护代码块、数学公式等特殊内容
- **并行处理**：支持多线程并行翻译，提高处理速度
- **进度保存**：定期自动保存翻译进度，防止意外中断
- **格式优化**：自动优化翻译后的文档格式

## 安装

### 从源代码编译

```bash
# 克隆仓库
git clone https://github.com/nerdneilsfield/go-translator-agent.git
cd go-translator-agent

# 构建项目
go build -o translator ./cmd/translator

# 安装到系统路径（可选）
go install ./cmd/translator
```

## 配置

翻译工具默认在 `~/.translator.yaml` 寻找配置文件。一个典型的配置文件如下：

```yaml
source_lang: "English"
target_lang: "Chinese"
country: "China"
default_model_name: "gpt-3.5-turbo"
active_step_set: "basic"
max_tokens_per_chunk: 2000
use_cache: true
cache_dir: "~/.translator-cache"
debug: false

# 性能和超时设置
request_timeout: 300 # 请求超时时间（秒）
translation_timeout: 1800 # 整体翻译超时时间（秒）
auto_save_interval: 300 # 自动保存间隔（秒）
concurrency: 4 # 并行翻译请求数

# 文本分割设置
min_split_size: 100 # 最小分割大小（字符数）
max_split_size: 1000 # 最大分割大小（字符数）
retry_failed_parts: true # 是否重试失败的部分
filter_reasoning: true # 过滤推理过程

# Markdown相关配置
post_process_markdown: true # 默认开启Markdown后处理
fix_math_formulas: true # 修复数学公式
fix_table_format: true # 修复表格格式
fix_mixed_content: true # 修复混合内容（中英文混合）
fix_picture: true # 修复图片链接和说明

models:
  gpt-3.5-turbo:
    name: "gpt-3.5-turbo"
    model_id: "gpt-3.5-turbo"
    api_type: "openai"
    base_url: ""
    key: "YOUR_OPENAI_API_KEY" 
    max_output_tokens: 4096
    max_input_tokens: 4096
    temperature: 0.7

step_sets:
  basic:
    id: "basic"
    name: "基本翻译"
    description: "基本的三步翻译过程"
    initial_translation:
      name: "初始翻译"
      model_name: "gpt-3.5-turbo"
      temperature: 0.5
    reflection:
      name: "反思"
      model_name: "gpt-3.5-turbo"
      temperature: 0.3
    improvement:
      name: "改进"
      model_name: "gpt-3.5-turbo"
      temperature: 0.5
    fast_mode_threshold: 300
```

## 使用方法

基本翻译命令：

```bash
translator 输入文件.md 输出文件.md
```

指定语言：

```bash
translator --source English --target Chinese document.md translated_document.md
```

使用特定步骤集：

```bash
translator --step-set quality document.md translated_document.md
```

设置并行度：

```bash
translator --concurrency 8 document.md translated_document.md
```

设置分块大小：

```bash
translator --min-split-size 200 --max-split-size 2000 document.md translated_document.md
```

设置自动保存间隔：

```bash
translator --auto-save-interval 600 document.md translated_document.md
```

禁用Markdown后处理：

```bash
translator --no-post-process document.md translated_document.md
```

仅格式化文件：

```bash
translator --format-only document.md formatted_document.md
```

显示帮助：

```bash
translator --help
```

列出可用模型：

```bash
translator --list-models
```

禁用缓存：

```bash
translator --cache=false document.md translated_document.md
```

## 支持的格式

- Markdown (*.md, *.markdown)
  - 智能分块和并行翻译
  - 自动保护代码块和数学公式
  - 保持原始格式和结构
  - 自动格式化和优化
  - 修复表格和图片格式
  - 优化中英文混排
- 纯文本 (*.txt)
  - 智能分段和并行翻译
  - 保持原始段落结构
  - 自动合并翻译结果
- EPUB (*.epub) - 计划支持
- LaTeX (*.tex) - 计划支持

## 高级特性

### 智能分块

系统会根据文档结构智能分块：
- 优先按自然段落分割
- 在句子边界处分割长段落
- 自动识别中英文标点符号
- 保持特殊内容的完整性

### 并行处理

支持多线程并行翻译：
- 可配置并行度
- 自动负载均衡
- 错误重试机制
- 进度跟踪和报告

### 自动保护

自动识别和保护特殊内容：
- 代码块和行内代码
- 数学公式
- 表格结构
- 图片链接
- 自定义占位符

### 进度保存

定期自动保存翻译进度：
- 可配置保存间隔
- 断点续传支持
- 临时文件管理
- 错误恢复机制

## 许可证

[MIT License](LICENSE)
