# Translation Providers

This package provides implementations of various translation services that can be used with the translation package.

## Supported Providers

### 1. OpenAI
- **Features**: LLM-based translation with GPT models
- **Supports**: Multi-step translation, custom instructions, streaming (V2)
- **API Key**: Required (OPENAI_API_KEY)
- **Versions**: 
  - V1: Custom implementation (legacy)
  - V2: Using official OpenAI Go SDK (recommended) ✨

### 2. Google Translate
- **Features**: Professional machine translation
- **Supports**: 100+ languages, HTML format preservation
- **API Key**: Required (GOOGLE_API_KEY)

### 3. DeepL
- **Features**: High-quality neural machine translation
- **Supports**: 30+ languages, formality settings
- **API Key**: Required (DEEPL_API_KEY)

### 4. DeepLX
- **Features**: Free DeepL alternative (self-hosted)
- **Supports**: Same languages as DeepL
- **API Key**: Not required

### 5. LibreTranslate
- **Features**: Open-source machine translation
- **Supports**: 17+ languages, self-hostable
- **API Key**: Optional (depends on server)

## Usage

### Basic Translation

```go
import (
    "github.com/nerdneilsfield/go-translator-agent/pkg/providers/openai"
    "github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// Configure provider
config := openai.DefaultConfig()
config.APIKey = "your-api-key"
provider := openai.New(config)

// Translate text
req := &translation.ProviderRequest{
    Text:           "Hello, world!",
    SourceLanguage: "English",
    TargetLanguage: "Chinese",
}

resp, err := provider.Translate(context.Background(), req)
if err != nil {
    log.Fatal(err)
}

fmt.Println(resp.Text)
```

### Mixed Providers with Translation Package

```go
import (
    "github.com/nerdneilsfield/go-translator-agent/pkg/providers/deepl"
    "github.com/nerdneilsfield/go-translator-agent/pkg/providers/openai"
    "github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

// Create providers
deeplConfig := deepl.DefaultConfig()
deeplConfig.APIKey = "deepl-key"
deeplProvider := deepl.New(deeplConfig)

openaiConfig := openai.DefaultConfig()
openaiConfig.APIKey = "openai-key"
openaiProvider := openai.New(openaiConfig)

// Configure translation with mixed providers
config := &translation.Config{
    SourceLanguage: "English",
    TargetLanguage: "Chinese",
    Steps: []translation.StepConfig{
        {
            Name:     "initial",
            Provider: "deepl",  // Use DeepL for initial translation
        },
        {
            Name:     "refine",
            Provider: "openai", // Use OpenAI for refinement
            Model:    "gpt-4",
        },
    },
}

// Create translator
translator, err := translation.New(config,
    translation.WithProviders(map[string]translation.TranslationProvider{
        "deepl":  deeplProvider,
        "openai": openaiProvider,
    }),
)
```

## Provider Configuration

### OpenAI

**推荐使用 V2 版本（基于官方 SDK）：**

```go
// V2 - 使用官方 OpenAI Go SDK (推荐)
config := openai.DefaultConfigV2()
config.APIKey = "sk-..."
config.Model = "gpt-4"
config.OrgID = "org-..." // 可选

provider := openai.NewV2(config)

// 支持流式响应
chunks, _ := provider.StreamTranslate(ctx, req)
for chunk := range chunks {
    fmt.Print(chunk.Text)
}
```

**旧版本（将被弃用）：**

```go
// V1 - 自定义实现
config := openai.Config{
    BaseConfig: providers.BaseConfig{
        APIKey:     "sk-...",
        APIEndpoint: "https://api.openai.com/v1",
        Timeout:    30 * time.Second,
        MaxRetries: 3,
    },
    Model:       "gpt-3.5-turbo",
    Temperature: 0.3,
    MaxTokens:   4096,
}
```

### Google Translate

```go
config := google.Config{
    BaseConfig: providers.BaseConfig{
        APIKey: "your-google-api-key",
    },
    ProjectID: "your-project-id", // Optional for Google Cloud
}
```

### DeepL

```go
config := deepl.Config{
    BaseConfig: providers.BaseConfig{
        APIKey: "your-deepl-key",
    },
    UseFreeAPI: false, // Set to true for free API
}
```

### DeepLX

```go
config := deeplx.Config{
    BaseConfig: providers.BaseConfig{
        APIEndpoint: "http://localhost:1188/translate",
    },
    AccessToken: "optional-token",
}
```

### LibreTranslate

```go
config := libretranslate.Config{
    BaseConfig: providers.BaseConfig{
        APIEndpoint: "https://libretranslate.com",
        APIKey:     "optional-key",
    },
    RequiresAPIKey: false,
}
```

## Environment Variables

Providers can be configured using environment variables:

- `OPENAI_API_KEY`: OpenAI API key
- `GOOGLE_API_KEY`: Google Translate API key
- `DEEPL_API_KEY`: DeepL API key
- `DEEPL_FREE_API`: Set to "true" to use DeepL free API
- `DEEPLX_ENDPOINT`: DeepLX server endpoint
- `DEEPLX_TOKEN`: Optional DeepLX access token
- `LIBRETRANSLATE_ENDPOINT`: LibreTranslate server endpoint
- `LIBRETRANSLATE_API_KEY`: Optional LibreTranslate API key

## Running the Example

```bash
# Basic translation
go run examples/main.go -provider=openai -text="Hello" -target="Spanish"

# Show provider capabilities
go run examples/main.go -provider=deepl -caps

# Three-step translation with OpenAI
go run examples/main.go -provider=openai -three-step -text="Complex text to translate"
```

## Provider Capabilities

Each provider has different capabilities:

| Provider | Max Text Length | Batch Support | HTML Support | Multi-Step | Free Tier |
|----------|----------------|---------------|--------------|------------|-----------|
| OpenAI   | 8,000          | No            | Yes          | Yes        | No        |
| Google   | 5,000          | Yes           | Yes          | No         | Limited   |
| DeepL    | 130,000        | Yes           | Yes          | No         | Limited   |
| DeepLX   | 5,000          | No            | No           | No         | Yes       |
| LibreTranslate | 5,000   | No            | Yes          | No         | Yes       |

## Error Handling

All providers implement retry logic for transient errors:

```go
provider := openai.New(config)

resp, err := provider.Translate(ctx, req)
if err != nil {
    var provErr *providers.Error
    if errors.As(err, &provErr) {
        fmt.Printf("Provider error: %s (code: %s)\n", provErr.Message, provErr.Code)
        if provErr.IsRetryable() {
            // Error was retried but still failed
        }
    }
}
```

## Health Checks

All providers support health checks:

```go
if err := provider.HealthCheck(ctx); err != nil {
    log.Printf("Provider unhealthy: %v", err)
}
```

## Extending Providers

To add a new provider, implement the `Provider` interface:

```go
type Provider interface {
    translation.TranslationProvider
    Configure(config interface{}) error
    GetCapabilities() Capabilities
    HealthCheck(ctx context.Context) error
}
```

See existing implementations for examples.