# 翻译工具 (Go-Translator-Agent)

一个高质量、灵活的多语言翻译系统，采用三步翻译流程确保翻译质量。

## 功能特点

- **三步翻译流程**：初始翻译、反思、改进
- **多模型支持**：可配置不同阶段使用不同的语言模型
- **多格式支持**：Markdown、纯文本，计划支持EPUB、LaTeX
- **格式保留**：保持原始文档的格式、结构和特殊元素
- **灵活缓存**：避免重复翻译，提高效率和一致性
- **命令行界面**：便于批处理和脚本集成

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

models:
  - model_name: "gpt-3.5-turbo"
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
translator --source English --target Spanish document.md translated_document.md
```

使用特定步骤集：

```bash
translator --step-set quality document.md translated_document.md
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
- 纯文本 (*.txt)
- EPUB (*.epub) - 计划支持
- LaTeX (*.tex) - 计划支持

## 许可证

[MIT License](LICENSE)
