# Translation Package

A standalone, production-ready Go library for multi-step text translation with LLM support. This package implements a sophisticated three-step translation process (initial translation, reflection, and improvement) that can be easily integrated into any Go project.

## Features

- **Standalone Library**: No external configuration framework dependencies
- **Three-Step Translation**: Implements initial translation, reflection, and improvement phases
- **Flexible Configuration**: Explicit configuration through structs and option functions
- **Smart Text Chunking**: Intelligent text splitting that preserves semantic integrity
- **Concurrent Processing**: Configurable parallel processing for better performance
- **Caching Support**: Built-in caching interface for translation results
- **Progress Tracking**: Real-time progress updates and callbacks
- **Metrics Collection**: Comprehensive translation metrics and statistics
- **Error Handling**: Robust error handling with retries for transient failures
- **LLM Agnostic**: Works with any LLM that implements the client interface
- **Professional Translation Services**: Support for DeepL, Google Translate, and other translation APIs
- **Mixed Provider Support**: Combine different providers for different translation steps

## Installation

```bash
go get github.com/nerdneilsfield/go-translator-agent/pkg/translation
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    
    "github.com/nerdneilsfield/go-translator-agent/pkg/translation"
)

func main() {
    // 1. Create configuration
    config := translation.DefaultConfig()
    config.SourceLanguage = "English"
    config.TargetLanguage = "Chinese"
    
    // 2. Create LLM client (implement the LLMClient interface)
    llmClient := NewYourLLMClient()
    
    // 3. Create translation service
    translator, err := translation.New(config,
        translation.WithLLMClient(llmClient),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // 4. Translate text
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

## Configuration

### Basic Configuration

```go
config := &translation.Config{
    SourceLanguage: "English",
    TargetLanguage: "Chinese",
    ChunkSize:      1000,      // Characters per chunk
    ChunkOverlap:   100,       // Overlap between chunks
    MaxConcurrency: 3,         // Parallel processing limit
    MaxRetries:     3,         // Retry attempts for failures
    EnableCache:    true,      // Enable caching
    Steps: []translation.StepConfig{
        // Define your translation steps
    },
}
```

### Translation Steps

The package supports configurable translation steps. The default configuration includes three steps:

1. **Initial Translation**: Direct translation from source to target language
2. **Reflection**: Analysis of the translation to identify potential improvements
3. **Improvement**: Final refinement based on the reflection feedback

Each step can use different providers (LLMs or professional translation services):

```go
steps := []translation.StepConfig{
    {
        Name:        "initial_translation",
        Provider:    "deepl",        // Use DeepL for initial translation
        Model:       "deepl-pro",
        Temperature: 0.3,
        MaxTokens:   4096,
        Prompt:      "Translate from {{source_language}} to {{target_language}}: {{text}}",
        SystemRole:  "You are a professional translator.",
    },
    {
        Name:        "reflection",
        Provider:    "openai",       // Use OpenAI for reflection
        Model:       "gpt-4",
        Temperature: 0.1,
        Prompt:      "Review this translation and provide feedback...",
    },
    // Add more steps as needed
}
```

## Provider Integration

### Using Translation Providers

The package supports both LLMs and professional translation services through a unified interface:

```go
// Configure with multiple providers
translator, err := translation.New(config,
    translation.WithProviders(map[string]translation.TranslationProvider{
        "deepl": deeplProvider,
        "openai": openaiProvider,
        "google": googleProvider,
    }),
)
```

### Implementing Translation Provider

To add support for a translation service, implement the `TranslationProvider` interface:

```go
type TranslationProvider interface {
    Translate(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error)
    GetName() string
    SupportsSteps() bool  // Return true if provider supports multi-step translation
}
```

Example DeepL implementation:

```go
type DeepLProvider struct {
    apiKey string
}

func (d *DeepLProvider) Translate(ctx context.Context, req *translation.ProviderRequest) (*translation.ProviderResponse, error) {
    // Call DeepL API
    resp, err := d.callDeepLAPI(req.Text, req.SourceLanguage, req.TargetLanguage)
    if err != nil {
        return nil, err
    }
    
    return &translation.ProviderResponse{
        Text:  resp.TranslatedText,
        Model: "deepl-pro",
    }, nil
}
```

### Implementing LLM Client

For backward compatibility, you can still implement the `LLMClient` interface:

```go
type LLMClient interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    GetModel() string
    HealthCheck(ctx context.Context) error
}
```

## Advanced Usage

### Mixed Provider Usage

Combine different providers for optimal results - use professional services for initial translation and LLMs for refinement:

```go
// Create providers
deeplProvider := &DeepLProvider{apiKey: "your-deepl-key"}
openaiProvider := &OpenAIProvider{apiKey: "your-openai-key"}

// Configure with mixed providers
config := &translation.Config{
    SourceLanguage: "English",
    TargetLanguage: "Chinese",
    Steps: []translation.StepConfig{
        {
            Name:     "initial_translation",
            Provider: "deepl",    // Professional translation
        },
        {
            Name:     "reflection",
            Provider: "openai",   // LLM for analysis
            Model:    "gpt-4",
        },
        {
            Name:     "improvement",
            Provider: "openai",   // LLM for refinement
            Model:    "gpt-4",
        },
    },
}

translator, err := translation.New(config,
    translation.WithProviders(map[string]translation.TranslationProvider{
        "deepl":  deeplProvider,
        "openai": openaiProvider,
    }),
)
```

### With Caching

```go
translator, err := translation.New(config,
    translation.WithLLMClient(llmClient),
    translation.WithCache(myCache), // Implement Cache interface
)
```

### With Progress Tracking

```go
translator, err := translation.New(config,
    translation.WithLLMClient(llmClient),
    translation.WithProgressCallback(func(p *translation.Progress) {
        fmt.Printf("Progress: %.2f%% - %s\n", p.Percent, p.Current)
    }),
)
```

### With Metrics Collection

```go
translator, err := translation.New(config,
    translation.WithLLMClient(llmClient),
    translation.WithMetrics(metricsCollector), // Implement MetricsCollector interface
)
```

### Batch Translation

```go
requests := []*translation.Request{
    {Text: "Hello"},
    {Text: "World"},
}

responses, err := translator.TranslateBatch(ctx, requests)
```

### Custom Text Chunking

```go
// Use default chunker
chunker := translation.NewDefaultChunker(1000, 100)

// Use smart chunker (preserves code blocks and lists)
smartChunker := translation.NewSmartChunker(1000, 100)

translator, err := translation.New(config,
    translation.WithLLMClient(llmClient),
    translation.WithChunker(smartChunker),
)
```

## Interfaces

### Service Interface

```go
type Service interface {
    Translate(ctx context.Context, req *Request) (*Response, error)
    TranslateBatch(ctx context.Context, reqs []*Request) ([]*Response, error)
    GetConfig() *Config
}
```

### Cache Interface

```go
type Cache interface {
    Get(ctx context.Context, key string) (string, bool, error)
    Set(ctx context.Context, key string, value string) error
    Delete(ctx context.Context, key string) error
    Clear(ctx context.Context) error
}
```

### MetricsCollector Interface

```go
type MetricsCollector interface {
    RecordTranslation(metrics *TranslationMetrics)
    RecordStep(metrics *StepMetrics)
    RecordError(err error, context map[string]string)
    GetSummary() *MetricsSummary
}
```

## Error Handling

The package provides comprehensive error handling:

```go
result, err := translator.Translate(ctx, req)
if err != nil {
    var transErr *translation.TranslationError
    if errors.As(err, &transErr) {
        log.Printf("Translation error: %s (code: %s)", transErr.Message, transErr.Code)
        if transErr.IsRetryable() {
            // Handle retryable error
        }
    }
}
```

## Best Practices

1. **Chunk Size**: Choose chunk sizes based on your LLM's context window and the nature of your text
2. **Concurrency**: Set `MaxConcurrency` based on your LLM's rate limits and system resources
3. **Caching**: Implement caching for production use to avoid redundant translations
4. **Error Handling**: Always handle errors and implement appropriate retry logic
5. **Monitoring**: Use metrics collection to monitor translation performance and costs

## Examples

See the [examples directory](examples/) for complete working examples:

- [Basic Usage](examples/basic/main.go) - Simple translation example
- [Mixed Providers](examples/mixed_providers/main.go) - Combining DeepL with OpenAI
- Advanced Usage - Complex configurations and custom implementations (coming soon)
- Integration Examples - Integration with popular LLM providers (coming soon)

## Performance Considerations

- **Text Chunking**: Large texts are automatically split into manageable chunks
- **Parallel Processing**: Chunks are processed concurrently up to `MaxConcurrency`
- **Caching**: Implement caching to avoid re-translating identical content
- **Connection Pooling**: Reuse LLM client connections when possible

## Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

## License

This package is part of the go-translator-agent project. See the main project for license information.