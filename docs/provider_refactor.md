# Provider 接口重构计划

## 背景

当前设计中使用 `LLMClient` 作为翻译提供商接口，但这个名称太具体了。实际上，翻译服务可能包括：

1. **LLM 服务**：OpenAI、Anthropic、本地 LLM 等
2. **专业翻译服务**：DeepL、Google Translate、百度翻译等
3. **混合服务**：可能同时支持多种模式

## 重构方案

### 1. 新的接口层次

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

// Capabilities 提供商能力
type Capabilities struct {
    SupportsBatch      bool     // 支持批量翻译
    SupportsStreaming  bool     // 支持流式响应
    SupportedLanguages []string // 支持的语言
    MaxTextLength      int      // 最大文本长度
    RequiresSteps      bool     // 是否需要多步骤（如LLM）
}
```

### 2. 具体实现

#### LLM 提供商（需要三步翻译）
```go
type LLMProvider struct {
    client LLMClient
    model  string
}

func (p *LLMProvider) Translate(ctx context.Context, req *TranslationRequest) (*TranslationResponse, error) {
    // 如果请求包含步骤配置，执行对应的提示词
    // 否则执行默认翻译
}
```

#### 直接翻译提供商（DeepL 等）
```go
type DeepLProvider struct {
    apiKey string
}

func (p *DeepLProvider) Translate(ctx context.Context, req *TranslationRequest) (*TranslationResponse, error) {
    // 直接调用 DeepL API
    // 忽略步骤配置，因为 DeepL 不需要
}
```

### 3. 适配现有代码

为了保持向后兼容，可以：

1. 保留 `LLMClient` 接口
2. 创建适配器将 `LLMClient` 包装为 `TranslationProvider`
3. 逐步迁移到新接口

```go
// LLMProviderAdapter 将 LLMClient 适配为 TranslationProvider
type LLMProviderAdapter struct {
    client LLMClient
}

func NewLLMProviderAdapter(client LLMClient) TranslationProvider {
    return &LLMProviderAdapter{client: client}
}
```

### 4. 配置更新

```go
type Config struct {
    // ... 其他字段
    
    // ProviderType 提供商类型
    ProviderType string // "llm", "deepl", "google", etc.
    
    // ProviderConfig 提供商特定配置
    ProviderConfig map[string]interface{}
}
```

## 实施步骤

1. **第一步**：定义新接口（不破坏现有代码）
2. **第二步**：实现适配器
3. **第三步**：添加新的提供商实现
4. **第四步**：更新文档和示例
5. **第五步**：标记旧接口为废弃（给用户迁移时间）

## 优势

1. **更通用**：支持各种翻译服务
2. **更灵活**：不同提供商可以有不同的实现策略
3. **更清晰**：接口名称更准确地反映功能
4. **向后兼容**：通过适配器保持兼容性

## 示例使用

```go
// 使用 LLM
llmProvider := translation.NewLLMProvider(openaiClient)
translator := translation.New(config, translation.WithProvider(llmProvider))

// 使用 DeepL
deeplProvider := translation.NewDeepLProvider(apiKey)
translator := translation.New(config, translation.WithProvider(deeplProvider))

// 使用多个提供商（fallback）
multiProvider := translation.NewMultiProvider(
    llmProvider,
    deeplProvider,
)
translator := translation.New(config, translation.WithProvider(multiProvider))
```