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

## Recent Updates

### Progress Bar Optimization & Enhanced Debugging (2025-06-18)

- **进度条更新频率优化**: Improved progress bar update frequency from 100ms to 50ms throttling for better visibility with increased logging
- **实时进度回调**: Implemented comprehensive progress callback system with real-time updates during translation rounds:
  - Group-level progress tracking during concurrent translation processing
  - Round-by-round progress updates (initial translation, retry rounds)
  - Node-level completion tracking with detailed status messages
- **改进的进度显示**: Enhanced progress display with contextual messages showing current operation status
- **线程安全的进度更新**: Thread-safe progress callback mechanism for concurrent translation operations

### Translation Failure Debugging & Cache Management (2025-06-18)

- **Network Timeout Fix**: Increased default provider timeout from 30 seconds to 5 minutes to support long-running LLM requests
- **Reasoning Marker Removal**: Enhanced `<think>` tag removal in translation chain execution paths with safer processing
- **Markdown Protection**: Added comprehensive protection for Markdown images (`![alt](url)`) and links (`[text](url)`) during translation
- **Cache Refresh**: Implemented `--refresh-cache` functionality to clear translation cache before starting new translations

### Detailed Translation Round Tracking (2025-06-18)

- **Multi-Round Statistics**: Track and display detailed statistics for each translation round (initial + retries)
- **Node-Level Tracking**: Record which specific nodes succeed/fail in each round with node IDs
- **Failure Analysis**: Comprehensive error type classification (timeout, network, auth, rate limit, etc.)
- **Visual Reporting**: Rich console output with emoji icons and structured failure reports showing:
  - Document-level statistics (total nodes, success rate, total rounds)
  - Round-by-round breakdown (which nodes succeeded/failed in each attempt)
  - Final failure details with error categorization and node previews
  - Performance metrics (duration per round, error distribution)

### Key File Locations for Recent Changes

- **`internal/translator/progress_bar.go`**: Progress bar throttling optimization (100ms → 50ms)
- **`internal/translator/batch_translator.go`**: Progress callback system, group-level progress tracking, and detailed round tracking
- **`internal/translator/coordinator.go`**: Progress callback integration, cache refresh functionality, and detailed summary reporting
- **`pkg/providers/provider.go`**: Network timeout configuration (30s → 5min)
- **`pkg/translation/chain.go`**: Reasoning marker removal in Provider and LLM execution paths  
- **`internal/cli/root.go`**: Cache refresh flag handling and detailed output display

## Development Notes

- The project uses Go modules with toolchain 1.23.4
- Dependencies managed through `go.mod`/`go.sum`
- Code formatting enforced with `gofumpt` and `gci`
- Linting with `golangci-lint` (config in `.golangci.yml`)
- Version information injected during build via ldflags
- Supports both Makefile and Justfile for build automation

## Coding Guidelines

- Use English to commit