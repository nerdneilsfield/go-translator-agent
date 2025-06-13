# Adapter Package

The `adapter` package provides a compatibility layer between the old `pkg/translator` interface and the new `pkg/translation` implementation. This allows existing code to continue working while we gradually migrate to the new architecture.

## Purpose

This adapter layer:
- Maintains backward compatibility with existing CLI and format processors
- Converts old Viper-based configuration to the new standalone configuration format
- Bridges the old `Translator` interface to the new `Service` interface
- Handles provider creation and management

## Components

### TranslatorAdapter

The main adapter that implements the old `pkg/translator.Translator` interface while using the new `pkg/translation.Service` internally.

```go
// Create adapter
adapter, err := adapter.NewTranslatorAdapter(cfg, logger)

// Use like old translator
result, err := adapter.Translate(text, retryFailedParts)
```

### Configuration Conversion

Converts the old Viper-based config structure to the new translation config:

```go
// Convert config
translationConfig, err := adapter.ConvertConfig(oldConfig)
```

### Provider Factory

Creates translation providers based on configuration:

```go
// Create providers
providers, err := adapter.CreateProviders(cfg)

// Or use factory
factory := adapter.NewProviderFactory(cfg)
provider, err := factory.CreateProvider("openai")
```

## Migration Path

1. **Phase 1** (Current): Use adapter for all existing code
2. **Phase 2**: Gradually update components to use new interfaces directly
3. **Phase 3**: Remove adapter layer once migration is complete

## Supported Features

- ✅ Three-step translation (initial, reflection, improvement)
- ✅ Multiple provider support (OpenAI, DeepL, Google, etc.)
- ✅ Progress tracking (limited compatibility)
- ✅ Configuration conversion
- ✅ Error handling and conversion
- ⚠️  Logging (simplified, no zap fields)
- ⚠️  Progress tracking (limited API compatibility)

## Known Limitations

1. **Progress Tracking**: The old progress tracker has a different API. Some methods like `MarkAsComplete()` are not available.
2. **Logging**: Logger integration is simplified to avoid zap field dependencies.
3. **Metrics**: Full metrics support requires using the new interfaces directly.

## Example Usage

```go
package main

import (
    "github.com/nerdneilsfield/go-translator-agent/internal/adapter"
    "github.com/nerdneilsfield/go-translator-agent/internal/config"
    "github.com/nerdneilsfield/go-translator-agent/internal/logger"
)

func main() {
    // Load old config
    cfg, err := config.LoadConfig("")
    if err != nil {
        panic(err)
    }
    
    // Create logger
    log := logger.NewLogger(cfg.Debug)
    
    // Create adapter
    translator, err := adapter.NewTranslatorAdapter(cfg, log)
    if err != nil {
        panic(err)
    }
    
    // Initialize
    translator.InitTranslator()
    defer translator.Finish()
    
    // Translate
    result, err := translator.Translate("Hello, world!", false)
    if err != nil {
        panic(err)
    }
    
    fmt.Println(result)
}
```