# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based multilingual translation system that uses a three-step translation process (initial translation, reflection, improvement) with multiple LLM support. The system supports multiple file formats (Markdown, plain text, EPUB) and includes intelligent text chunking, parallel processing, caching, and progress tracking.

## Common Development Commands

Build the project:
```bash
make build
# or
just build
```

Run tests with coverage:
```bash
make test
# or  
just test
```

Format code:
```bash
make fmt
# or
just fmt
```

Lint code:
```bash
make lint
# or
just lint
```

Run the application:
```bash
make run
# or
just run [args]
```

Install the binary:
```bash
make install
# or
just install
```

Clean build artifacts:
```bash
make clean
# or
just clean
```

## Architecture Overview

### Core Components

- **`cmd/translator/`**: Main CLI application entry point
- **`internal/cli/`**: CLI command definitions and handling using Cobra
- **`internal/config/`**: Configuration management with Viper (YAML/TOML support)
- **`internal/logger/`**: Structured logging with Zap
- **`pkg/translator/`**: Core translation engine and interfaces
- **`pkg/formats/`**: File format processors (Markdown, Text, EPUB, HTML)
- **`pkg/progress/`**: Progress tracking and reporting

### Key Interfaces

- **`Translator`** (`pkg/translator/interfaces.go`): Main translation interface
- **`LLMClient`** (`pkg/translator/interfaces.go`): Language model client abstraction
- **`Cache`** (`pkg/translator/interfaces.go`): Caching interface for translation results
- **`Processor`** (`pkg/formats/format.go`): File format processing interface

### Translation Flow

1. **Text Processing**: Input text is intelligently chunked based on natural boundaries
2. **Three-Step Translation**: 
   - Initial translation with specified model
   - Reflection phase to identify issues
   - Improvement phase to refine translation
3. **Format Preservation**: Original document structure and formatting are maintained
4. **Parallel Processing**: Multiple chunks processed concurrently with configurable concurrency
5. **Progress Tracking**: Real-time progress with auto-save functionality
6. **Caching**: Translation results cached to avoid reprocessing

### Configuration System

- Default config location: `~/.translator.yaml`
- Supports multiple model configurations (OpenAI-compatible APIs)
- Step sets allow different models for different translation phases
- Configurable parameters: concurrency, timeouts, chunk sizes, caching

### Testing Structure

- **`tests/`**: Integration and end-to-end tests
- Tests are organized by functionality (translator_tests/, html/, etc.)
- Use `make test` to run all tests with coverage reporting

## Development Notes

- The project uses Go modules with toolchain 1.23.4
- Dependencies managed through `go.mod`/`go.sum`
- Code formatting enforced with `gofumpt` and `gci`
- Linting with `golangci-lint` (config in `.golangci.yml`)
- Version information injected during build via ldflags
- Supports both Makefile and Justfile for build automation