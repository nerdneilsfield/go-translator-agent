# Go-Translator-Agent 重构计划

## 一、重构目标和原则

### 核心目标
1. **独立可复用**: 将 translator 设计为可被其他项目引用的独立库
2. **职责分离**: 将 pkg/translator 巨型包拆分为多个职责单一的包
3. **接口细化**: 创建细粒度、可组合的接口，替代大而全的接口
4. **统计体系**: 建立完整的翻译统计和性能指标系统
5. **包边界清晰**: 明确区分公共API (pkg) 和内部实现 (internal)
6. **可扩展性**: 便于添加新的LLM提供商、格式处理器、缓存实现

### 设计原则
- **零外部依赖**: translator 包不依赖特定配置框架（如 viper）
- **依赖倒置**: 高层模块依赖抽象，而非具体实现
- **接口隔离**: 客户端不应依赖不需要的接口
- **开闭原则**: 对扩展开放，对修改关闭
- **单一职责**: 每个包和接口只负责一个职责
- **向后兼容**: 通过适配器层保证现有API继续工作

## 二、新架构设计

### 分层架构
```
┌─────────────────────────────────────────────────┐
│                   CLI Layer                      │
│              (cmd/translator)                    │
├─────────────────────────────────────────────────┤
│               Application Layer                  │
│         (internal/cli, internal/config)          │
├─────────────────────────────────────────────────┤
│                Domain Layer                      │
│    (pkg/translation, pkg/llm, pkg/metrics)       │
├─────────────────────────────────────────────────┤
│             Infrastructure Layer                 │
│   (internal/translator, internal/llm, etc.)      │
└─────────────────────────────────────────────────┘
```

### 核心模块关系
```
translation.Service
    ├── llm.Client
    ├── cache.Cache
    ├── metrics.Collector
    ├── progress.Tracker
    └── formats.Processor
```

## 三、独立 Translator 包设计

### 设计理念
`pkg/translation` 将被设计为一个完全独立的、可被其他 Go 项目引用的库，专注于提供三步翻译功能。

### 配置方式
不依赖任何外部配置框架，使用显式的配置结构体：

```go
// Config 翻译器配置
type Config struct {
    // 基础配置
    SourceLanguage string
    TargetLanguage string
    
    // 分块配置
    ChunkSize       int
    ChunkOverlap    int
    MaxConcurrency  int
    
    // 翻译步骤配置
    Steps []StepConfig
}

// StepConfig 单个翻译步骤配置
type StepConfig struct {
    Name        string            // 步骤名称
    Model       string            // 使用的模型
    Temperature float32           // 温度参数
    MaxTokens   int              // 最大token数
    Prompt      string           // 提示词模板
    Variables   map[string]string // 提示词变量
}

// 使用示例
config := &translation.Config{
    SourceLanguage: "English",
    TargetLanguage: "Chinese",
    ChunkSize:      1000,
    MaxConcurrency: 3,
    Steps: []translation.StepConfig{
        {
            Name:  "initial_translation",
            Model: "gpt-4",
            Prompt: "Translate the following {{source}} text to {{target}}...",
        },
        {
            Name:  "reflection",
            Model: "gpt-4",
            Prompt: "Review this translation and identify issues...",
        },
        {
            Name:  "improvement",
            Model: "gpt-4",
            Prompt: "Improve the translation based on feedback...",
        },
    },
}
```

### 依赖注入
所有外部依赖通过接口注入，不硬编码实现：

```go
// New 创建翻译服务
func New(config *Config, opts ...Option) (*Service, error) {
    // 通过选项模式注入依赖
}

// 选项函数
func WithLLMClient(client llm.Client) Option
func WithCache(cache cache.Cache) Option
func WithMetrics(collector metrics.Collector) Option
func WithProgress(tracker progress.Tracker) Option
```

### 使用示例
其他项目可以轻松集成：

```go
import (
    "github.com/yourusername/go-translator-agent/pkg/translation"
    "github.com/yourusername/go-translator-agent/pkg/llm/openai"
)

// 创建 LLM 客户端
llmClient := openai.NewClient("your-api-key")

// 创建翻译器
translator, err := translation.New(config,
    translation.WithLLMClient(llmClient),
    translation.WithCache(myCache),
)

// 执行翻译
result, err := translator.Translate(ctx, "Hello, world!")
```

## 四、详细包结构

```
go-translator-agent/
├── pkg/                              # 公共API包（可被外部项目引用）
│   ├── translation/                  # 独立的翻译库
│   │   ├── interfaces.go             # 核心翻译接口定义
│   │   ├── service.go                # 翻译服务实现
│   │   ├── chain.go                  # 三步翻译链实现
│   │   ├── chunker.go                # 文本分块器
│   │   ├── config.go                 # 配置结构体
│   │   ├── types.go                  # 请求/响应类型
│   │   ├── errors.go                 # 错误定义
│   │   ├── options.go                # 选项函数
│   │   └── examples/                 # 使用示例
│   │       └── basic/                # 基础示例
│   │
│   ├── providers/                    # 翻译提供商接口
│   │   ├── interfaces.go             # Provider, TranslationProvider接口
│   │   ├── types.go                  # 请求/响应类型
│   │   ├── config.go                 # 提供商配置
│   │   ├── capabilities.go           # 能力定义
│   │   └── errors.go                 # 提供商错误
│   │
│   ├── metrics/                      # 统计和指标接口
│   │   ├── interfaces.go             # Collector, Reporter接口
│   │   ├── types.go                  # Metrics类型定义
│   │   ├── events.go                 # 事件定义
│   │   └── aggregator.go             # 聚合器接口
│   │
│   ├── cache/                        # 缓存接口
│   │   ├── interfaces.go             # Cache接口
│   │   ├── types.go                  # CacheKey, CacheEntry
│   │   └── options.go                # 缓存配置
│   │
│   ├── formats/                      # 格式处理（重构现有）
│   │   ├── interfaces.go             # Processor, Parser接口
│   │   ├── registry.go               # 格式注册表
│   │   └── types.go                  # Document, Block类型
│   │
│   └── progress/                     # 进度跟踪接口
│       ├── interfaces.go             # Tracker, Reporter接口
│       ├── types.go                  # Progress, Stage类型
│       └── events.go                 # 进度事件
│
├── internal/                         # 内部实现（应用特定）
│   ├── app/                          # 应用层适配器
│   │   ├── translator.go             # 包装 pkg/translation 的应用适配器
│   │   ├── config_adapter.go         # 将 viper 配置转换为 translation.Config
│   │   └── factory.go                # 创建和配置翻译器实例
│   │
│   ├── providers/                    # 提供商实现
│   │   ├── llm/                      # LLM提供商
│   │   │   ├── openai/               # OpenAI实现
│   │   │   ├── anthropic/            # Anthropic实现
│   │   │   └── adapter.go            # LLM适配器
│   │   ├── translation/              # 专业翻译服务
│   │   │   ├── deepl/                # DeepL实现
│   │   │   ├── google/               # Google Translate实现
│   │   │   └── baidu/                # 百度翻译实现
│   │   ├── factory.go                # 提供商工厂
│   │   └── multi.go                  # 多提供商支持
│   │
│   ├── metrics/                      # 指标收集实现
│   │   ├── collector.go              # 默认收集器
│   │   ├── reporter.go               # 报告器实现
│   │   ├── storage/                  # 存储后端
│   │   │   ├── memory.go             # 内存存储
│   │   │   └── file.go               # 文件存储
│   │   └── exporters/                # 导出器
│   │       ├── json.go               # JSON导出
│   │       └── prometheus.go         # Prometheus导出
│   │
│   ├── cache/                        # 缓存实现
│   │   ├── file.go                   # 文件系统缓存
│   │   ├── memory.go                 # 内存缓存
│   │   ├── redis.go                  # Redis缓存
│   │   └── multi.go                  # 多级缓存
│   │
│   ├── formats/                      # 格式处理实现
│   │   ├── markdown/                 # Markdown处理
│   │   ├── epub/                     # EPUB处理
│   │   ├── html/                     # HTML处理
│   │   └── text/                     # 纯文本处理
│   │
│   └── progress/                     # 进度系统实现
│       ├── tracker.go                # 进度跟踪器
│       ├── reporter.go               # 进度报告器
│       └── persistence.go            # 进度持久化
│
├── cmd/translator/                   # CLI应用
├── tests/                            # 测试代码（重组）
├── configs/                          # 配置文件
└── docs/                             # 文档
```

## 五、核心接口设计示例

### 1. 翻译包配置 (pkg/translation/config.go)
```go
// Config 翻译器独立配置，不依赖外部配置框架
type Config struct {
    // 语言配置
    SourceLanguage string `json:"source_language"`
    TargetLanguage string `json:"target_language"`
    
    // 分块配置
    ChunkSize      int `json:"chunk_size"`
    ChunkOverlap   int `json:"chunk_overlap"`
    MaxConcurrency int `json:"max_concurrency"`
    
    // 重试配置
    MaxRetries     int           `json:"max_retries"`
    RetryDelay     time.Duration `json:"retry_delay"`
    
    // 翻译步骤
    Steps []StepConfig `json:"steps"`
}

// StepConfig 翻译步骤配置
type StepConfig struct {
    Name        string            `json:"name"`
    Model       string            `json:"model"`
    Temperature float32           `json:"temperature"`
    MaxTokens   int              `json:"max_tokens"`
    Timeout     time.Duration     `json:"timeout"`
    Prompt      string            `json:"prompt"`
    Variables   map[string]string `json:"variables"`
}

// Validate 验证配置
func (c *Config) Validate() error {
    // 验证配置的合法性
}
```

### 2. 翻译接口 (pkg/translation/interfaces.go)
```go
// Service 高层翻译服务接口
type Service interface {
    // Translate 执行完整的翻译流程
    Translate(ctx context.Context, req *Request) (*Response, error)
    
    // TranslateFile 翻译文件
    TranslateFile(ctx context.Context, input, output string, opts ...Option) error
    
    // GetSupportedFormats 获取支持的格式
    GetSupportedFormats() []string
}

// Translator 单步翻译器接口
type Translator interface {
    // TranslateText 翻译文本
    TranslateText(ctx context.Context, text string, opts ...Option) (string, error)
    
    // TranslateBatch 批量翻译
    TranslateBatch(ctx context.Context, texts []string, opts ...Option) ([]string, error)
}

// Chain 翻译链接口
type Chain interface {
    // Execute 执行翻译链
    Execute(ctx context.Context, input string) (*ChainResult, error)
    
    // AddStep 添加翻译步骤
    AddStep(step Step) Chain
}
```

### 2. 提供商接口 (pkg/providers/interfaces.go)
```go
// Provider 翻译提供商基础接口
type Provider interface {
    // GetName 获取提供商名称
    GetName() string
    
    // GetCapabilities 获取提供商能力
    GetCapabilities() Capabilities
    
    // HealthCheck 健康检查
    HealthCheck(ctx context.Context) error
}

// TranslationProvider 翻译提供商接口
type TranslationProvider interface {
    Provider
    
    // Translate 执行翻译
    Translate(ctx context.Context, req *TranslationRequest) (*TranslationResponse, error)
}

// BatchProvider 批量翻译提供商接口
type BatchProvider interface {
    TranslationProvider
    
    // TranslateBatch 批量翻译
    TranslateBatch(ctx context.Context, reqs []*TranslationRequest) ([]*TranslationResponse, error)
}

// StreamProvider 流式翻译提供商接口
type StreamProvider interface {
    TranslationProvider
    
    // TranslateStream 流式翻译
    TranslateStream(ctx context.Context, req *TranslationRequest) (<-chan TranslationChunk, error)
}
```

### 3. 统计接口 (pkg/metrics/interfaces.go)
```go
// Collector 指标收集器接口
type Collector interface {
    // RecordTranslation 记录翻译指标
    RecordTranslation(metrics *TranslationMetrics)
    
    // RecordError 记录错误
    RecordError(err error, context map[string]string)
    
    // GetSummary 获取统计摘要
    GetSummary() *Summary
}

// TranslationMetrics 翻译指标
type TranslationMetrics struct {
    ID              string
    StartTime       time.Time
    Duration        time.Duration
    SourceLanguage  string
    TargetLanguage  string
    InputTokens     int
    OutputTokens    int
    Model           string
    Success         bool
    ErrorType       string
    CacheHit        bool
}
```

## 六、作为独立库使用

### 安装
```bash
go get github.com/yourusername/go-translator-agent/pkg/translation
go get github.com/yourusername/go-translator-agent/pkg/llm/openai
```

### 基础使用示例
```go
package main

import (
    "context"
    "log"
    
    "github.com/yourusername/go-translator-agent/pkg/translation"
    "github.com/yourusername/go-translator-agent/pkg/llm/openai"
)

func main() {
    // 配置翻译器
    config := &translation.Config{
        SourceLanguage: "English",
        TargetLanguage: "Chinese",
        ChunkSize:      1000,
        MaxConcurrency: 3,
        Steps: []translation.StepConfig{
            {
                Name:  "translate",
                Model: "gpt-4",
                Prompt: "Translate from {{source}} to {{target}}:\n\n{{text}}",
            },
            {
                Name:  "reflect",
                Model: "gpt-4",
                Prompt: "Review and identify issues in this translation...",
            },
            {
                Name:  "improve",
                Model: "gpt-4",
                Prompt: "Improve the translation based on the reflection...",
            },
        },
    }
    
    // 创建 LLM 客户端
    llmClient := openai.NewClient(openai.Config{
        APIKey: "your-api-key",
    })
    
    // 创建翻译服务
    translator, err := translation.New(config,
        translation.WithLLMClient(llmClient),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // 执行翻译
    result, err := translator.Translate(context.Background(), 
        &translation.Request{
            Text: "Hello, world!",
        })
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Translation: %s", result.Text)
}
```

### 高级功能
```go
// 使用缓存
translator, err := translation.New(config,
    translation.WithLLMClient(llmClient),
    translation.WithCache(myCache),
    translation.WithMetrics(metricsCollector),
)

// 自定义进度回调
translator, err := translation.New(config,
    translation.WithLLMClient(llmClient),
    translation.WithProgressCallback(func(p *translation.Progress) {
        log.Printf("Progress: %d/%d chunks", p.Completed, p.Total)
    }),
)
```

## 七、迁移计划

### 第一阶段：基础架构（1-2周）
1. 创建新的包结构和接口定义
2. 实现核心接口的基础版本
3. 建立测试框架
4. 创建兼容层包装器

### 第二阶段：核心功能迁移（2-3周）
1. 实现 pkg/translation 完整功能（自包含的翻译库）
2. 重构LLM客户端到新结构
3. 实现新的缓存系统
4. 集成指标收集系统
5. 创建应用层适配器（internal/app）

### 第三阶段：格式处理优化（1-2周）
1. 重构格式处理器接口
2. 优化各格式实现
3. 改进格式自动检测

### 第四阶段：完善和优化（1周）
1. 性能优化
2. 完善错误处理
3. 增强日志和调试信息
4. 更新所有测试

### 第五阶段：文档和清理（1周）
1. 更新所有文档
2. 创建迁移指南
3. 标记废弃API
4. 清理冗余代码

## 八、风险评估与缓解

### 主要风险
1. **破坏现有功能**: 通过完整的测试覆盖和渐进式迁移缓解
2. **性能退化**: 建立性能基准测试，持续监控
3. **API不兼容**: 提供兼容层，给用户充足迁移时间
4. **复杂度增加**: 通过清晰的文档和示例代码降低学习成本

### 缓解措施
- 保持 pkg/translator 的兼容性包装器至少6个月
- 每个阶段都进行全面的回归测试
- 提供详细的迁移文档和工具
- 建立性能监控基准

## 九、预期收益

1. **更好的可维护性**: 清晰的包边界和职责分离
2. **更强的可扩展性**: 易于添加新功能和实现
3. **完善的监控**: 内置的指标收集和统计系统
4. **更好的测试性**: 细粒度接口便于单元测试
5. **标准化的错误处理**: 统一的错误类型和处理流程
6. **性能优化空间**: 更灵活的缓存和并发控制

## 十、下一步行动

1. 审查并批准此重构计划
2. 创建重构分支
3. 开始第一阶段的接口定义工作
4. 建立自动化测试和CI流程
5. 逐步实施各阶段计划

这个重构计划旨在将 go-translator-agent 提升到企业级项目标准，同时保持其易用性和灵活性。通过渐进式的迁移策略，我们可以在不影响现有用户的情况下完成整个重构过程。

## 十一、最新进展更新（2025年1月13日）

### 已完成的里程碑 ✅

#### 1. 核心 translation 包
- **完全独立**: 无 viper 或其他外部配置依赖
- **接口设计**: Service、Chain、Step、LLMClient 等核心接口
- **三步翻译**: 支持配置化的多步骤翻译流程
- **智能分块**: 保持代码块、列表等结构完整性
- **完整测试**: 100% 核心功能测试覆盖

#### 2. 提供商生态系统
- **Provider 接口**: 统一的提供商抽象
- **5 个实现**:
  ```
  ├── OpenAI (v1: 自定义实现)
  ├── OpenAI (v2: 官方 SDK + 流式支持) ✨
  ├── Google Translate (100+ 语言)
  ├── DeepL (专业翻译)
  ├── DeepLX (免费替代)
  └── LibreTranslate (开源方案)
  ```
- **混合模式**: 支持不同步骤使用不同提供商

#### 3. 创新功能
- **流式翻译**: OpenAI v2 支持实时流式输出
- **灵活组合**: 如 DeepL 初译 + GPT-4 润色
- **提供商注册表**: 动态注册和发现机制

### 下一阶段实施计划（1月14-21日）

#### 第一周：适配层和 CLI（1月14-17日）

**Day 1-2: 适配层架构**
```
internal/
├── adapter/
│   ├── translator.go    # 核心适配器
│   ├── config.go        # 配置转换器
│   └── factory.go       # 提供商工厂
└── app/
    └── translator.go    # 应用层封装
```

**Day 3: CLI 迁移**
- 保持命令行接口不变
- 内部调用新的 translation 包
- 添加 `--provider` 选项支持

**Day 4: 集成测试**
- 验证新旧系统行为一致
- 性能基准对比

#### 第二周：格式处理器和优化（1月18-21日）

**Day 5-6: 格式处理器迁移**
- Markdown → 新接口
- Text → 新接口
- EPUB/HTML → 新接口

**Day 7: 缓存和监控**
- 实现 FileCache
- 添加 Prometheus 指标

**Day 8: 文档和发布**
- 更新用户文档
- 创建迁移指南
- 准备 v2.0 发布

### 技术架构决策

#### 1. 配置兼容性
```yaml
# 旧格式（继续支持）
models:
  default: gpt-3.5-turbo
  steps:
    - name: translate
      model: gpt-4

# 新格式（推荐）
translation:
  provider: openai  # 或 mixed
  steps:
    - name: initial
      provider: deepl
    - name: improve
      provider: openai
      model: gpt-4
```

#### 2. 依赖注入策略
```go
// 应用层使用依赖注入容器
type App struct {
    translator translation.Service
    cache      cache.Cache
    metrics    metrics.Collector
}

// 通过 Wire 或手动注入
func NewApp(cfg *config.Config) (*App, error) {
    // 根据配置创建依赖
}
```

#### 3. 特性开关
```go
// 环境变量控制
USE_NEW_TRANSLATOR=true  # 启用新系统
TRANSLATOR_DEBUG=true    # 详细日志
```

### 风险缓解措施

1. **回滚计划**
   - Git 分支策略：feature/translator-v2
   - 保留旧实现至少 3 个版本
   - 快速切换机制

2. **性能保证**
   - 基准测试：每个 PR 必须通过
   - 内存分析：避免额外分配
   - 并发测试：验证线程安全

3. **用户沟通**
   - 提前发布 Beta 版本
   - 详细的升级指南
   - 社区反馈渠道

### 成功指标

- [ ] 所有现有测试通过
- [ ] 性能提升 10%+（得益于更好的并发）
- [ ] 零 Breaking Changes（用户端）
- [ ] 新功能采用率 > 30%（3个月内）

### 长期愿景

这次重构为未来功能奠定基础：
- **插件系统**: 用户自定义提供商
- **Web API**: RESTful/gRPC 服务
- **更多提供商**: Anthropic、Baidu、ChatGLM
- **高级功能**: 术语库、翻译记忆库