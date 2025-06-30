# 步骤集 V2 使用指南

## 概述

步骤集 V2 是 go-translator-agent 的新一代翻译流程配置系统，提供了更灵活、更强大的翻译步骤定义能力。与旧版本固定的三步翻译流程不同，V2 允许：

- **灵活的步骤数量**：不再限制为三步，可以根据需要定义任意数量的步骤
- **混合提供商支持**：在同一个翻译流程中使用不同的翻译提供商
- **并行步骤执行**：支持多个步骤并行执行，然后综合结果
- **自定义流程**：根据具体需求设计专属的翻译流程

## 配置格式

### 基本结构

```yaml
step_sets_v2:
  your_step_set_id:
    id: your_step_set_id
    name: 步骤集名称
    description: 步骤集描述
    steps:
      - name: step_name
        provider: provider_name
        model_name: model_id
        temperature: 0.5
        max_tokens: 4096
        timeout: 60
        prompt: "提示词模板"
        system_role: "系统角色描述"
        variables:
          custom_var: value
    fast_mode_threshold: 300
```

### 字段说明

#### 步骤集字段
- `id`: 步骤集的唯一标识符
- `name`: 步骤集的显示名称
- `description`: 步骤集的详细描述
- `steps`: 步骤列表，定义翻译流程中的每个步骤
- `fast_mode_threshold`: 快速模式阈值（字符数），低于此值时可能跳过某些步骤

#### 步骤字段
- `name`: 步骤名称，在流程中必须唯一
- `provider`: 翻译提供商（openai, deepl, google, deeplx, libretranslate, anthropic, ollama）
- `model_name`: 使用的模型名称
- `temperature`: 温度参数（0-2），控制输出的随机性
- `max_tokens`: 最大输出令牌数
- `timeout`: 超时时间（秒）
- `prompt`: 提示词模板，支持变量替换
- `system_role`: 系统角色描述
- `variables`: 自定义变量，会合并到提示词变量中

## 提供商支持

### 1. OpenAI
- 支持所有 GPT 模型
- 需要配置 API 密钥
- 支持自定义端点（兼容 API）

### 2. DeepL
- 专业翻译服务
- 需要 DeepL API 密钥
- 适合高质量直译

### 3. DeepLX
- DeepL 的免费替代
- 需要本地运行 DeepLX 服务
- 无需 API 密钥

### 4. Google Translate
- Google 翻译 API
- 需要 Google Cloud API 密钥
- 支持大量语言对

### 5. LibreTranslate
- 开源翻译服务
- 可自托管或使用公共实例
- 部分实例需要 API 密钥

### 6. Anthropic (计划中)
- Claude 系列模型
- 高质量的理解和生成能力

### 7. Ollama (本地模型)
- 支持本地运行的 LLM
- 无需 API 密钥
- 适合隐私敏感的翻译任务

## 使用示例

### 1. 基础三步翻译

传统的三步翻译流程，使用单一 LLM：

```yaml
step_sets_v2:
  basic:
    id: basic
    name: 基本翻译
    description: 使用单一模型的三步翻译过程
    steps:
      - name: initial_translation
        provider: openai
        model_name: gpt-3.5-turbo
        temperature: 0.5
        prompt: |
          Translate the following {{source}} text to {{target}}:
          {{text}}
      
      - name: reflection
        provider: openai
        model_name: gpt-3.5-turbo
        temperature: 0.3
        prompt: |
          Review this translation:
          Original: {{original_text}}
          Translation: {{translation}}
      
      - name: improvement
        provider: openai
        model_name: gpt-3.5-turbo
        temperature: 0.5
        prompt: |
          Improve the translation based on feedback:
          Original: {{original_text}}
          Translation: {{translation}}
          Feedback: {{feedback}}
```

### 2. 混合提供商模式

结合专业翻译服务和 AI 优化：

```yaml
step_sets_v2:
  professional_hybrid:
    id: professional_hybrid
    name: 专业混合翻译
    description: DeepL + GPT-4 优化
    steps:
      - name: deepl_translation
        provider: deepl
        model_name: deepl
        prompt: "{{text}}"
      
      - name: ai_optimization
        provider: openai
        model_name: gpt-4
        temperature: 0.3
        prompt: |
          优化这个专业翻译，使其更加自然：
          
          原文：{{original_text}}
          DeepL翻译：{{deepl_translation}}
          
          请保持准确性的同时，让译文更符合{{target}}的表达习惯。
```

### 3. 多引擎对比模式

使用多个翻译引擎并综合最佳结果：

```yaml
step_sets_v2:
  multi_engine:
    id: multi_engine
    name: 多引擎对比
    description: 综合多个翻译结果
    steps:
      # 并行执行多个翻译
      - name: deepl_result
        provider: deepl
        model_name: deepl
        prompt: "{{text}}"
      
      - name: google_result
        provider: google
        model_name: google-translate
        prompt: "{{text}}"
      
      - name: libre_result
        provider: libretranslate
        model_name: libretranslate
        prompt: "{{text}}"
      
      # AI 综合评估
      - name: synthesis
        provider: openai
        model_name: gpt-4
        temperature: 0.2
        prompt: |
          比较以下翻译并生成最佳版本：
          
          1. DeepL: {{deepl_result}}
          2. Google: {{google_result}}
          3. LibreTranslate: {{libre_result}}
          
          综合各版本的优点，生成最准确自然的翻译。
```

### 4. 快速模式

仅使用专业翻译服务，适合大量文本：

```yaml
step_sets_v2:
  fast:
    id: fast
    name: 快速翻译
    description: 仅使用 DeepL
    steps:
      - name: translation
        provider: deepl
        model_name: deepl
        prompt: "{{text}}"
    fast_mode_threshold: 10000
```

### 5. 本地隐私模式

使用本地模型，保护数据隐私：

```yaml
step_sets_v2:
  local_private:
    id: local_private
    name: 本地隐私翻译
    description: 使用 Ollama 本地模型
    steps:
      - name: translation
        provider: ollama
        model_name: llama2
        temperature: 0.3
        prompt: |
          Translate from {{source}} to {{target}}:
          {{text}}
      
      - name: refinement
        provider: ollama
        model_name: llama2
        temperature: 0.2
        prompt: |
          Refine this translation:
          {{translation}}
```

## 变量系统

步骤集支持丰富的变量系统，用于在提示词中动态插入内容：

### 系统变量
- `{{text}}`: 当前要翻译的文本
- `{{original_text}}`: 原始文本
- `{{source}}`: 源语言
- `{{target}}`: 目标语言
- `{{previous_step_name}}`: 引用前一步骤的输出

### 自定义变量
可以在步骤配置中定义自定义变量：

```yaml
steps:
  - name: custom_step
    variables:
      tone: "formal"
      domain: "technical"
    prompt: |
      Translate in a {{tone}} tone for {{domain}} domain:
      {{text}}
```

## 最佳实践

### 1. 选择合适的提供商组合
- **高质量需求**：DeepL/Google + GPT-4 优化
- **成本敏感**：DeepLX/LibreTranslate + GPT-3.5
- **隐私要求**：本地 Ollama 模型
- **速度优先**：单步专业翻译服务

### 2. 优化步骤顺序
- 将成本较低的步骤放在前面
- 并行执行独立的翻译步骤
- 在最后进行综合和优化

### 3. 设置合理的参数
- `temperature`：翻译任务通常使用 0.3-0.5
- `max_tokens`：根据文本长度合理设置
- `timeout`：为网络服务预留足够时间

### 4. 利用快速模式阈值
- 对于短文本，可以跳过复杂的多步骤流程
- 设置合理的阈值以平衡质量和效率

## 从旧格式迁移

系统会自动将旧格式的步骤集转换为新格式。如果你有现有的配置：

```yaml
# 旧格式
step_sets:
  old_set:
    initial_translation:
      model_name: gpt-3.5-turbo
      temperature: 0.5
    reflection:
      model_name: gpt-3.5-turbo
      temperature: 0.3
    improvement:
      model_name: gpt-3.5-turbo
      temperature: 0.5
```

它会被自动转换为新格式，但建议手动迁移以利用新功能。

## 命令行使用

### 列出可用的步骤集

```bash
translator --list-step-sets
```

### 使用特定步骤集

```bash
translator --step-set mixed_professional input.txt output.txt
```

### 指定单一提供商（临时）

```bash
translator --provider deepl input.txt output.txt
```

## 故障排除

### 1. 提供商连接失败
- 检查 API 密钥配置
- 验证网络连接
- 确认服务端点正确

### 2. 步骤超时
- 增加 `timeout` 设置
- 检查网络延迟
- 考虑使用更快的提供商

### 3. 变量未替换
- 确保变量名正确
- 检查步骤间的依赖关系
- 验证前置步骤成功执行

## 未来计划

- 支持条件步骤（根据条件执行不同分支）
- 步骤结果缓存和重用
- 更多提供商集成（Anthropic、Cohere 等）
- 步骤执行的可视化监控
- 自动步骤优化建议