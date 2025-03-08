# 翻译工具项目文档

## 1. 产品需求文档 (PRD)

### 1.1 产品概述

#### 1.1.1 产品介绍

翻译工具是一个高质量、灵活的多语言翻译系统，采用三步翻译流程来确保翻译质量。该工具支持多种文件格式，可以为不同翻译阶段配置不同的语言模型，并提供完善的缓存机制以提高效率。

#### 1.1.2 目标用户

- 专业翻译人员
- 内容创作者和发布者
- 需要高质量翻译的技术文档作者
- 需要批量处理多语言内容的组织机构

#### 1.1.3 核心特性

- 三步翻译流程：初始翻译、反思、改进
- 多模型支持：可配置不同阶段使用不同的语言模型
- 多格式支持：Markdown、EPUB、纯文本、LaTeX
- 格式保留：保持原始文档的格式、结构和特殊元素
- 灵活缓存：避免重复翻译，提高效率和一致性
- 命令行界面：便于批处理和脚本集成
- 可扩展设计：易于添加新的格式和语言模型支持

### 1.2 功能需求

#### 1.2.1 三步翻译流程

1. **初始翻译**：将源文本翻译成目标语言
2. **反思**：分析初始翻译的质量，提出改进建议
3. **改进**：根据反思结果优化初始翻译

#### 1.2.2 模型配置

- 支持为每个翻译步骤配置不同的LLM模型
- 支持多种LLM提供商：OpenAI、Anthropic、Mistral等
- 每个模型可配置不同的API密钥和基础URL
- 可调整温度、最大令牌数等参数

#### 1.2.3 文件格式处理

- **Markdown**：保留Markdown语法和结构
- **EPUB**：处理电子书格式，保留章节结构
- **纯文本**：保留段落和基本格式
- **LaTeX**：保留数学公式、命令和环境

#### 1.2.4 缓存系统

- 基于文件的持久化缓存
- 缓存键基于源文本、源语言、目标语言和步骤集
- 可配置缓存目录
- 可禁用缓存

#### 1.2.5 命令行界面

- 指定源文件和目标文件
- 选择源语言和目标语言
- 指定翻译步骤集
- 配置缓存行为
- 查看支持的模型和格式

### 1.3 非功能需求

#### 1.3.1 性能

- 大型文档支持文本分块处理
- 翻译速度取决于选择的语言模型和文本大小

#### 1.3.2 安全性

- API密钥安全存储和处理
- 避免在日志中泄露敏感信息

#### 1.3.3 可扩展性

- 易于添加新的语言模型支持
- 易于添加新的文件格式处理器

#### 1.3.4 可用性

- 详细的命令行帮助信息
- 丰富的日志输出，支持调试
- 清晰的错误信息

### 1.4 技术要求

#### 1.4.1 开发语言和框架

- 使用Go语言开发
- 使用Cobra和Viper处理命令行和配置
- 使用Zap进行日志记录

#### 1.4.2 依赖项

- OpenAI、Anthropic、Mistral等API客户端库
- 文件格式处理库（EPUB、Markdown等）
- 测试框架

## 2. API文档

### 2.1 核心包API

#### 2.1.1 翻译器接口 (pkg/translator)

```go
// Translator提供从源语言到目标语言的翻译方法
type Translator interface {
    // Translate将文本从源语言翻译到目标语言
    Translate(text string) (string, error)
    
    // GetLogger返回与翻译器关联的日志记录器
    GetLogger() Logger
}

// New创建一个新的翻译器实例
func New(config *Config, options ...Option) (*Translator, error)
```

#### 2.1.2 配置API (pkg/translator)

```go
// Config保存翻译器的所有配置
type Config struct {
    SourceLang       string
    TargetLang       string
    Country          string
    DefaultModelName string
    ModelConfigs     map[string]ModelConfig
    StepSets         map[string]StepSetConfig
    ActiveStepSet    string
    MaxTokensPerChunk int
    CacheDir         string
    UseCache         bool
    Debug            bool
}

// LoadConfig从文件加载配置
func LoadConfig(configPath string) (*Config, error)

// SaveConfig将配置保存到文件
func SaveConfig(config *Config, configPath string) error
```

#### 2.1.3 格式处理器API (pkg/formats)

```go
// Processor定义文件格式处理器的接口
type Processor interface {
    // TranslateFile翻译文件内容并写入输出文件
    TranslateFile(inputPath, outputPath string) error
    
    // TranslateText翻译文本内容并保留格式
    TranslateText(text string) (string, error)
    
    // GetName返回处理器的名称
    GetName() string
}

// NewProcessor创建指定格式的处理器
func NewProcessor(t Translator, format string) (Processor, error)

// ProcessorFromFilePath根据文件扩展名选择合适的处理器
func ProcessorFromFilePath(t Translator, filePath string) (Processor, error)

// RegisteredFormats返回支持的文件格式列表
func RegisteredFormats() []string
```

### 2.2 使用示例

#### 2.2.1 基本翻译

```go
import (
    "fmt"
    "github.com/nerdneilsfield/go-translator-agent/pkg/translator"
)

func main() {
    // 创建默认配置
    config := translator.NewDefaultConfig()
    config.SourceLang = "English"
    config.TargetLang = "Spanish"
    
    // 创建翻译器
    t, err := translator.New(config)
    if err != nil {
        panic(err)
    }
    
    // 执行翻译
    translated, err := t.Translate("Hello, world!")
    if err != nil {
        panic(err)
    }
    
    fmt.Println(translated) // 输出: "¡Hola, mundo!"
}
```

#### 2.2.2 文件翻译

```go
import (
    "github.com/nerdneilsfield/go-translator-agent/pkg/formats"
    "github.com/nerdneilsfield/go-translator-agent/pkg/translator"
)

func main() {
    // 创建配置
    config := translator.NewDefaultConfig()
    config.SourceLang = "English"
    config.TargetLang = "Spanish"
    
    // 创建翻译器
    t, err := translator.New(config)
    if err != nil {
        panic(err)
    }
    
    // 获取处理器
    processor, err := formats.ProcessorFromFilePath(t, "document.md")
    if err != nil {
        panic(err)
    }
    
    // 翻译文件
    err = processor.TranslateFile("document.md", "document_es.md")
    if err != nil {
        panic(err)
    }
}
```

## 3. 架构设计文档

### 3.1 整体架构

翻译工具采用三层架构：

1. **命令行界面层**：处理用户输入和配置
2. **核心业务逻辑层**：执行翻译流程
3. **基础设施层**：与外部服务交互（API、文件系统）

### 3.2 目录结构

```
translator/
├── cmd/                           # 命令行入口点
│   └── translator/                
│       └── main.go                # 主程序入口
│
├── internal/                      # 内部实现
│   ├── cli/                       # CLI相关代码
│   │   ├── root.go                # 主命令实现
│   │   ├── translate.go           # 翻译命令实现
│   │   └── config.go              # CLI配置管理
│   │
│   ├── config/                    # 配置管理
│   │   ├── config.go              # 配置读取和解析
│   │   ├── defaults.go            # 默认配置值
│   │   └── viper.go               # Viper适配器
│   │
│   └── logger/                    # 日志管理
│       └── logger.go              # 日志初始化和配置
│
├── pkg/                           # 公共API
│   ├── translator/                # 核心翻译库
│   │   ├── translator.go          # 翻译器实现
│   │   ├── interfaces.go          # 核心接口定义
│   │   ├── models.go              # 模型管理
│   │   ├── llm_clients.go         # LLM客户端实现
│   │   ├── cache.go               # 缓存实现
│   │   └── tokenizer.go           # 文本分块
│   │
│   └── formats/                   # 格式处理器
│       ├── format.go              # 格式接口
│       ├── markdown.go            # Markdown处理
│       ├── epub.go                # EPUB处理
│       ├── text.go                # 纯文本处理
│       └── latex.go               # LaTeX处理
│
├── configs/                       # 配置示例
├── go.mod                         # Go模块定义
└── go.sum                         # 依赖校验
```

### 3.3 核心组件

#### 3.3.1 翻译器 (Translator)

- 实现三步翻译流程
- 管理文本分块和处理
- 利用缓存减少API调用

#### 3.3.2 LLM客户端 (LLMClient)

- 提供统一的语言模型接口
- 支持多种模型提供商
- 处理API错误和重试逻辑

#### 3.3.3 缓存 (Cache)

- 存储和检索翻译结果
- 基于配置和内容生成缓存键
- 文件系统持久化

#### 3.3.4 格式处理器 (Processor)

- 解析特定格式文档
- 分离需要翻译的内容和保留的结构
- 重组翻译后的内容

### 3.4 关键流程

#### 3.4.1 翻译流程

1. 解析命令行参数和配置
2. 初始化翻译器和格式处理器
3. 读取源文件
4. 按格式解析和提取内容
5. 执行三步翻译流程
6. 重组翻译后的内容
7. 写入目标文件
8. 统计每个步骤消耗的 tokens，包括输入和输出

#### 3.4.2 三步翻译详细流程

1. **初始翻译**：
   - 检查缓存
   - 如有必要，分块文本
   - 发送API请求
   - 收集翻译结果

2. **反思**：
   - 分析初始翻译和源文本
   - 识别潜在问题
   - 生成改进建议

3. **改进**：
   - 基于反思结果优化翻译
   - 更新缓存
   - 返回最终翻译

### 3.5 翻译提示词 (Prompts)

#### 3.5.1 初始翻译提示词

```
This is an {source_lang} to {target_lang} translation, please provide the {target_lang} translation for this text. 
Do not provide any explanations or text apart from the translation.
{source_lang}: {source_text}

{target_lang}:
```

#### 3.5.2 反思提示词

```
Your task is to carefully read a source text and a translation from {source_lang} to {target_lang}, and then give constructive criticism and helpful suggestions to improve the translation. 
The final style and tone of the translation should match the style of {target_lang} colloquially spoken in {country}.

The source text and initial translation, delimited by XML tags <SOURCE_TEXT></SOURCE_TEXT> and <TRANSLATION></TRANSLATION>, are as follows:

<SOURCE_TEXT>
{source_text}
</SOURCE_TEXT>

<TRANSLATION>
{translation}
</TRANSLATION>

When writing suggestions, pay attention to whether there are ways to improve the translation's
(i) accuracy (by correcting errors of addition, mistranslation, omission, or untranslated text),
(ii) fluency (by applying {target_lang} grammar, spelling and punctuation rules, and ensuring there are no unnecessary repetitions),
(iii) style (by ensuring the translations reflect the style of the source text and take into account any cultural context),
(iv) terminology (by ensuring terminology use is consistent and reflects the source text domain; and by only ensuring you use equivalent idioms {target_lang}).

Write a list of specific, helpful and constructive suggestions for improving the translation.
Each suggestion should address one specific part of the translation.
Output only the suggestions and nothing else.
```

#### 3.5.3 改进提示词

```
Your task is to carefully read, then edit, a translation from {source_lang} to {target_lang}, taking into
account a list of expert suggestions and constructive criticisms.

The source text, the initial translation, and the expert linguist suggestions are delimited by XML tags <SOURCE_TEXT></SOURCE_TEXT>, <TRANSLATION></TRANSLATION> and <EXPERT_SUGGESTIONS></EXPERT_SUGGESTIONS> as follows:

<SOURCE_TEXT>
{source_text}
</SOURCE_TEXT>

<TRANSLATION>
{translation}
</TRANSLATION>

<EXPERT_SUGGESTIONS>
{reflection}
</EXPERT_SUGGESTIONS>

Please take into account the expert suggestions when editing the translation. Edit the translation by ensuring:

(i) accuracy (by correcting errors of addition, mistranslation, omission, or untranslated text),
(ii) fluency (by applying {target_lang} grammar, spelling and punctuation rules and ensuring there are no unnecessary repetitions),
(iii) style (by ensuring the translations reflect the style of the source text)
(iv) terminology (inappropriate for context, inconsistent use), or
(v) other errors.

Output only the new translation and nothing else.
```

#### 3.5.4 多块翻译提示词

对于大型文档，翻译器会将内容分割成多个块进行处理。以下是处理多块翻译的提示词：

**多块初始翻译**：

```
Your task is to provide a professional translation from {source_lang} to {target_lang} of PART of a text.

The source text is below, delimited by XML tags <SOURCE_TEXT> and </SOURCE_TEXT>. Translate only the part within the source text
delimited by <TRANSLATE_THIS> and </TRANSLATE_THIS>. You can use the rest of the source text as context, but do not translate any
of the other text. Do not output anything other than the translation of the indicated part of the text.

<SOURCE_TEXT>
{tagged_text}
</SOURCE_TEXT>

To reiterate, you should translate only this part of the text, shown here again between <TRANSLATE_THIS> and </TRANSLATE_THIS>:
<TRANSLATE_THIS>
{chunk_to_translate}
</TRANSLATE_THIS>

Output only the translation of the portion you are asked to translate, and nothing else.
```

**多块反思**：

```
Your task is to carefully read a source text and part of a translation of that text from {source_lang} to {target_lang}, and then give constructive criticism and helpful suggestions for improving the translation.
The final style and tone of the translation should match the style of {target_lang} colloquially spoken in {country}.

The source text is below, delimited by XML tags <SOURCE_TEXT> and </SOURCE_TEXT>, and the part that has been translated
is delimited by <TRANSLATE_THIS> and </TRANSLATE_THIS> within the source text. You can use the rest of the source text
as context for critiquing the translated part.

<SOURCE_TEXT>
{tagged_text}
</SOURCE_TEXT>

To reiterate, only part of the text is being translated, shown here again between <TRANSLATE_THIS> and </TRANSLATE_THIS>:
<TRANSLATE_THIS>
{chunk_to_translate}
</TRANSLATE_THIS>

The translation of the indicated part, delimited below by <TRANSLATION> and </TRANSLATION>, is as follows:
<TRANSLATION>
{translation_1_chunk}
</TRANSLATION>

When writing suggestions, pay attention to whether there are ways to improve the translation's:
(i) accuracy (by correcting errors of addition, mistranslation, omission, or untranslated text),
(ii) fluency (by applying {target_lang} grammar, spelling and punctuation rules, and ensuring there are no unnecessary repetitions),
(iii) style (by ensuring the translations reflect the style of the source text and take into account any cultural context),
(iv) terminology (by ensuring terminology use is consistent and reflects the source text domain; and by only ensuring you use equivalent idioms {target_lang}).

Write a list of specific, helpful and constructive suggestions for improving the translation.
Each suggestion should address one specific part of the translation.
Output only the suggestions and nothing else.
```

**多块改进**：

```
Your task is to carefully read, then improve, a translation from {source_lang} to {target_lang}, taking into
account a set of expert suggestions and constructive criticisms. Below, the source text, initial translation, and expert suggestions are provided.

The source text is below, delimited by XML tags <SOURCE_TEXT> and </SOURCE_TEXT>, and the part that has been translated
is delimited by <TRANSLATE_THIS> and </TRANSLATE_THIS> within the source text. You can use the rest of the source text
as context, but need to provide a translation only of the part indicated by <TRANSLATE_THIS> and </TRANSLATE_THIS>.

<SOURCE_TEXT>
{tagged_text}
</SOURCE_TEXT>

To reiterate, only part of the text is being translated, shown here again between <TRANSLATE_THIS> and </TRANSLATE_THIS>:
<TRANSLATE_THIS>
{chunk_to_translate}
</TRANSLATE_THIS>

The translation of the indicated part, delimited below by <TRANSLATION> and </TRANSLATION>, is as follows:
<TRANSLATION>
{translation_1_chunk}
</TRANSLATION>

The expert translations of the indicated part, delimited below by <EXPERT_SUGGESTIONS> and </EXPERT_SUGGESTIONS>, are as follows:
<EXPERT_SUGGESTIONS>
{reflection_chunk}
</EXPERT_SUGGESTIONS>

Taking into account the expert suggestions rewrite the translation to improve it, paying attention
to whether there are ways to improve the translation's

(i) accuracy (by correcting errors of addition, mistranslation, omission, or untranslated text),
(ii) fluency (by applying {target_lang} grammar, spelling and punctuation rules and ensuring there are no unnecessary repetitions),
(iii) style (by ensuring the translations reflect the style of the source text)
(iv) terminology (inappropriate for context, inconsistent use), or
(v) other errors.

Output only the new translation of the indicated part and nothing else.
```

## 4. 用户手册

### 4.1 安装指南

#### 4.1.1 从源代码安装

```bash
# 克隆仓库
git clone https://github.com/nerdneilsfield/go-translator-agent.git
cd translator

# 构建项目
go build -o translator ./cmd/translator

# 安装到系统路径（可选）
go install ./cmd/translator
```

#### 4.1.2 预编译二进制文件

```bash
# 下载适合您系统的二进制文件
# 解压并添加到PATH
chmod +x translator
sudo mv translator /usr/local/bin/
```

### 4.2 配置指南

#### 4.2.1 API密钥设置

在配置文件中设置：

```yaml
# ~/.translator.yaml
models:
      - model_name: "gpt-3.5"
        api_type: "openai"
        base_url: ""
        key: ""
        max_output_tokens: 8192
        max_input_tokens: 8192
        temprature: 0.6
```

#### 4.2.2 步骤集配置

创建自定义翻译步骤集：

```yaml
# ~/.translator.yaml
step_sets:
  my-custom:
    id: "my-custom"
    name: "My Custom Translation"
    description: "Custom translation workflow"
    initial_translation:
      name: "Initial"
      model_name: "gpt-4-turbo"
      temperature: 0.3
    reflection:
      name: "Reflection"
      model_name: "claude-3-opus"
      temperature: 0.4
    improvement:
      name: "Final"
      model_name: "mistral-large"
      temperature: 0.2
    fast_mode_threshold: 300
```

### 4.3 基本使用

#### 4.3.1 翻译文件

```bash
# 基本用法
translator document.md translated_document.md

# 指定语言
translator --source English --target Spanish document.md translated_document.md

# 使用特定步骤集
translator --step-set high-quality document.md translated_document.md

# 指定格式（当无法从扩展名自动检测）
translator --format markdown README.txt README_es.txt
```

#### 4.3.2 查看可用选项

```bash
# 显示帮助信息
translator --help

# 列出可用模型
translator --list-models

# 列出支持的文件格式
translator --list-formats

# 列出可用步骤集
translator --list-step-sets
```

### 4.4 高级用法

#### 4.4.1 缓存管理

```bash
# 禁用缓存
translator --cache=false --source English --target Spanish document.md translated_document.md

# 指定缓存目录
translator --cache-dir=/path/to/cache --source English --target Spanish document.md translated_document.md
```

#### 4.4.2 调试

```bash
# 启用调试日志
translator --debug --source English --target Spanish document.md translated_document.md
```

#### 4.4.3 批量处理

```bash
# 使用脚本批量处理
for file in *.md; do
  translator --source English --target Spanish "$file" "es/$file"
done
```

## 5. 开发指南

### 5.1 环境设置

#### 5.1.1 开发依赖

- Go 1.18+
- 开发工具: VSCode或GoLand

#### 5.1.2 设置开发环境

```bash
# 克隆仓库
git clone https://github.com/nerdneilsfield/go-translator-agent.git
cd translator

# 安装依赖
go mod download

# 运行测试
go test ./...
```

### 5.2 添加新功能

#### 5.2.1 添加新的模型支持

1. 在`pkg/translator/models.go`中添加新模型配置
2. 在`pkg/translator/llm_clients.go`中添加新客户端实现
3. 更新`DefaultModelConfigs()`函数添加新模型

#### 5.2.2 添加新的格式处理器

1. 在`pkg/formats/`下创建新处理器文件
2. 实现`Processor`接口
3. 在`pkg/formats/format.go`中注册新格式

### 5.3 最佳实践

#### 5.3.1 错误处理

- 使用具体的错误消息
- 包装错误以提供上下文
- 避免在翻译器中处理API错误

#### 5.3.2 日志记录

- 使用适当的日志级别
- 避免在生产级别记录敏感信息
- 包含足够上下文以便调试

#### 5.3.3 测试

- 为核心功能编写单元测试
- 使用模拟对象测试外部依赖
- 为命令行界面编写集成测试

## 6. 贡献指南

### 6.1 提交代码

#### 6.1.1 拉取请求流程

1. Fork仓库
2. 创建功能分支
3. 提交更改
4. 运行测试
5. 推送到Fork
6. 创建拉取请求

#### 6.1.2 代码风格

- 遵循Go标准代码风格
- 使用`gofmt`或`goimports`格式化代码
- 遵循包设计原则

### 6.2 报告问题

#### 6.2.1 Bug报告

- 提供清晰的步骤复现问题
- 包含相关日志输出
- 描述预期和实际行为

#### 6.2.2 功能请求

- 描述需求和用例
- 如可能，提供设计建议
- 解释为什么这个功能对项目有价值

## 7. 常见问题解答

### 7.1 一般问题

**Q: 支持哪些语言？**  
A: 支持所有由配置的语言模型支持的语言。OpenAI的GPT-4和Claude等模型支持多种语言。

**Q: 可以自定义翻译流程吗？**  
A: 是的，通过自定义步骤集可以调整每个翻译阶段使用的模型、参数和阈值。

**Q: 如何处理敏感或机密文档？**  
A: 所有内容都通过API发送到选定的语言模型。请查阅所用模型提供商的隐私政策。缓存的翻译内容存储在本地。

### 7.2 技术问题

**Q: 如何在大型文档上提高性能？**  
A: 工具会自动将大型文档分块处理。启用缓存以避免重新翻译相同内容。

**Q: 如果API调用失败怎么办？**  
A: 对于速率限制错误，工具会自动重试。对于其他错误，将提供详细的错误消息。

**Q: 如何删除缓存数据？**  

A: 缓存默认存储在`TEMDIR/translator-cache`目录。其中 `TEMDIR` 是系统相关的缓存储存目录。 可以安全删除此目录中的文件，或使用`--cache-dir`指定新位置。
