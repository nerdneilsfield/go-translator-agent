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

### Advanced Document Pre-Processing System (2025-06-18)

- **全新预处理框架**: Implemented comprehensive document pre-processing pipeline before translation
  - New workflow: `原始文件 → PreFormatter → 临时文件 → 现有格式化流程 → 翻译`
  - Automatic temporary file management with cleanup after translation
  - Intelligent content structure analysis and optimization
- **图片内容分离**: Intelligent image content separation for better translation
  - Detects `![](...)` image patterns and separates them into standalone paragraphs
  - Adds proper spacing before and after images for clean document structure
  - Handles image captions separately from image links
- **数学公式标准化**: Mathematical formula standardization
  - Converts irregular `$$...$$` patterns to standardized format: `$$\n公式内容\n$$`
  - Ensures proper spacing around formulas for optimal processing
  - Preserves formula content integrity during normalization
- **HTML表格智能转换**: Advanced HTML table to Markdown conversion
  - Detects `<html><body><table>` patterns and converts to standard Markdown tables
  - Preserves table structure and content during conversion
  - Wraps converted tables with protection markers to prevent translation
  - Uses goquery library for robust HTML parsing
- **裸链接自动包装**: Automatic bare link conversion to Markdown format
  - Converts bare URLs, DOI links, and arXiv references to `[url](url)` format
  - Intelligent detection to avoid double-processing existing Markdown links
  - Supports multiple URL patterns (HTTP/HTTPS, DOI, arXiv)
- **引用文献智能分离**: Intelligent reference separation and protection
  - Automatically detects and separates individual reference entries
  - Converts continuous reference blocks to individual lines
  - Protects entire reference section from translation with special markers
  - Handles various reference formatting patterns

### Critical Bug Fixes & Retry Mechanism Enhancement (2025-06-18)

- **重试机制修复**: Fixed critical bug where retry rounds had 0% success rate due to node reference issues
  - Root cause: `groupFailedNodesWithContext` was creating new NodeInfo objects instead of reusing original references
  - Fix: Use original node references so status updates are reflected in the main nodes array
- **上下文节点优化**: Optimized retry context strategy to reduce unnecessary overhead
  - Changed from max 2 nodes before/after (4 total) to max 1 node before/after (2 total)
  - Provides essential context while improving retry efficiency and reducing costs
- **相似度检查改进**: Enhanced similarity check logic to reduce false positives:
  - Now compares clean text (with protection markers removed) instead of protected text
  - Skip similarity check for short content (< 20 characters) as high similarity is normal
  - Added RemoveProtectionMarkers method to PreserveManager for accurate comparison
  - Improved verbose logging with clean text comparison details
- **错误分类增强**: Completely rewrote error classification system to properly handle TranslationError structures:
  - Support structured error codes (LLM_ERROR, NETWORK_ERROR, etc.) instead of only string matching
  - Recursive error unwrapping to extract nested error information
  - Proper mapping of error codes to user-friendly Chinese display names
- **翻译步骤级别错误追踪**: Added comprehensive step-level error tracking:
  - Track which translation step failed (initial_translation, reflection, improvement)
  - Display step index (第1步, 第2步, 第3步) in failure reports
  - Enhanced FailedNodeDetail structure with Step and StepIndex fields
- **改进的错误报告**: Enhanced failure reporting with step-specific information and better error classification

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

### Comprehensive Document Pre-Processing System (2025-06-18)

- **OCR Content Optimization**: Implemented comprehensive pre-processing pipeline to handle OCR-generated markdown issues:
  - **Image Separation**: Automatically separates inline images into standalone paragraphs with proper spacing
  - **Mathematical Formula Standardization**: Converts inline math formulas to multi-line format with proper spacing
  - **HTML Table Conversion**: Converts `<html><body><table>` structures to clean Markdown table format with protection markers
  - **Bare Link Wrapping**: Automatically converts bare URLs (http/https, DOI, arXiv) to proper Markdown link format
  - **Reference Citation Separation**: Processes reference sections with individual citation formatting and protection markers
- **Content Protection**: All processed elements are wrapped with protection markers to prevent translation interference
- **Intelligent Processing**: Only processes valid structures and skips malformed content appropriately
- **Temporary File Management**: Creates temporary pre-processed files with automatic cleanup
- **Comprehensive Logging**: Detailed debug logging for each processing step with match counts and statistics

### Key File Locations for Recent Changes

- **`internal/translator/progress_bar.go`**: Progress bar throttling optimization (100ms → 50ms)
- **`internal/translator/batch_translator.go`**: Progress callback system, group-level progress tracking, and detailed round tracking
- **`internal/translator/coordinator.go`**: Progress callback integration, cache refresh functionality, and detailed summary reporting
- **`pkg/providers/provider.go`**: Network timeout configuration (30s → 5min)
- **`pkg/translation/chain.go`**: Reasoning marker removal in Provider and LLM execution paths  
- **`internal/cli/root.go`**: Cache refresh flag handling, detailed output display, and preformat integration
- **`internal/preformat/preformatter.go`**: Comprehensive document pre-processing system for OCR-generated content

## Development Notes

- The project uses Go modules with toolchain 1.23.4
- Dependencies managed through `go.mod`/`go.sum`
- Code formatting enforced with `gofumpt` and `gci`
- Linting with `golangci-lint` (config in `.golangci.yml`)
- Version information injected during build via ldflags
- Supports both Makefile and Justfile for build automation

## Coding Guidelines

- Use English to commit