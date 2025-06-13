# 推理模型支持指南

## 概述

go-translator-agent 现在支持推理模型（Reasoning Models），这些模型在生成答案时会包含内部思考过程。系统能够自动识别并移除这些思考过程，只保留最终的翻译结果。

## 支持的推理模型

### 1. OpenAI o1 系列
- **o1-preview**: OpenAI 的顶级推理模型
- **o1-mini**: 更快速、更经济的推理模型
- **特点**: 内部推理过程已被 OpenAI 隐藏，API 返回的是清理后的结果

### 2. DeepSeek R1
- **deepseek-r1**: DeepSeek 的推理模型
- **特点**: 使用 `<think>` 和 `</think>` 标签包含思考过程
- **需要配置**: `reasoning_tags: ["<think>", "</think>"]`

### 3. QwQ 系列
- **qwq-32b**: 阿里通义千问的推理模型
- **特点**: 同样使用 `<think>` 和 `</think>` 标签
- **需要配置**: `reasoning_tags: ["<think>", "</think>"]`

## 配置方法

### 1. 模型配置

在配置文件中为推理模型添加必要的标记：

```yaml
models:
  # OpenAI o1 - 不需要特殊处理
  o1-preview:
    name: o1-preview
    model_id: o1-preview
    api_type: openai
    base_url: https://api.openai.com/v1
    key: ${OPENAI_API_KEY}
    is_reasoning: true  # 标记为推理模型
    # OpenAI 已经隐藏了推理过程，无需配置 reasoning_tags

  # DeepSeek R1 - 需要移除思考标签
  deepseek-r1:
    name: deepseek-r1
    model_id: deepseek-r1
    api_type: openai
    base_url: https://api.deepseek.com/v1
    key: ${DEEPSEEK_API_KEY}
    is_reasoning: true
    reasoning_tags: ["<think>", "</think>"]  # 指定推理标签

  # 自定义推理模型
  custom-reasoning:
    name: custom-reasoning
    model_id: custom-model-v1
    api_type: openai
    base_url: ${CUSTOM_API_BASE}
    key: ${CUSTOM_API_KEY}
    is_reasoning: true
    reasoning_tags: ["<thinking>", "</thinking>"]  # 自定义标签
```

### 2. 步骤集配置

创建使用推理模型的翻译步骤集：

```yaml
step_sets_v2:
  reasoning_translation:
    id: reasoning_translation
    name: 推理模型深度翻译
    description: 使用推理模型进行深度思考的高质量翻译
    steps:
      # 使用推理模型进行初始翻译
      - name: deep_translation
        provider: openai
        model_name: deepseek-r1
        temperature: 0.3
        max_tokens: 8192
        prompt: |
          请将以下{{source}}文本翻译为{{target}}。
          请深入思考原文的含义、文化背景和最佳表达方式。
          
          原文：
          {{text}}
        system_role: 你是一位专业的翻译专家。

      # 使用 GPT-4 进行质量评估
      - name: quality_check
        provider: openai
        model_name: gpt-4
        temperature: 0.2
        max_tokens: 2048
        prompt: |
          请评估这个翻译的质量：
          
          原文：{{original_text}}
          译文：{{deep_translation}}
          
          请指出任何可以改进的地方。
```

## 工作原理

### 1. 自动检测

系统会自动检测常见的推理标记：
- `<think>...</think>`
- `<thinking>...</thinking>`
- `<thought>...</thought>`
- `<reasoning>...</reasoning>`
- `<reflection>...</reflection>`
- `<internal>...</internal>`
- `[THINKING]...[/THINKING]`
- `[REASONING]...[/REASONING]`
- ` ```thinking...``` `
- ` ```reasoning...``` `

### 2. 处理流程

1. **模型输出**: 推理模型返回包含思考过程的完整响应
2. **检测**: 系统检查是否配置了 `is_reasoning: true`
3. **移除**: 根据配置的 `reasoning_tags` 或自动检测移除思考过程
4. **清理**: 清理多余的空行，保持格式整洁
5. **返回**: 返回清理后的纯翻译结果

### 3. 示例

输入（DeepSeek R1 的输出）：
```
<think>
让我仔细分析这个句子的含义...
"artificial intelligence" 应该翻译为"人工智能"
"changing" 表示正在进行的改变...
"our world" 指的是我们的世界...
</think>

人工智能正在改变我们的世界。
```

处理后的输出：
```
人工智能正在改变我们的世界。
```

## 最佳实践

### 1. 选择合适的推理模型

- **高质量需求**: 使用 o1-preview 或 deepseek-r1
- **快速响应**: 使用 o1-mini 或传统模型
- **成本敏感**: 考虑推理模型的额外计算成本

### 2. 优化提示词

推理模型特别适合需要深度思考的翻译任务：

```yaml
prompt: |
  请深入分析并翻译以下内容：
  
  1. 理解原文的深层含义和文化背景
  2. 考虑目标语言的表达习惯
  3. 确保专业术语的准确性
  4. 保持原文的语气和风格
  
  原文：{{text}}
```

### 3. 混合使用

可以在步骤集中混合使用推理模型和传统模型：

```yaml
steps:
  # 第一步：快速初译（传统模型）
  - name: quick_translation
    model_name: gpt-3.5-turbo
    temperature: 0.5
  
  # 第二步：深度优化（推理模型）
  - name: deep_optimization
    model_name: o1-mini
    temperature: 0.3
    prompt: |
      请深入思考并优化这个翻译：
      {{quick_translation}}
```

## 性能考虑

### 1. 响应时间

推理模型通常需要更长的响应时间：
- 传统模型：1-5 秒
- 推理模型：5-30 秒

### 2. 成本

推理模型的价格通常更高：
- 输入成本：3-15 倍于传统模型
- 输出成本：4-20 倍于传统模型

### 3. 质量提升

推理模型在以下方面表现更好：
- 复杂句子的理解
- 文化细节的把握
- 专业术语的准确性
- 语言风格的保持

## 故障排除

### 1. 推理标签未被移除

检查：
- 模型配置中 `is_reasoning: true` 是否设置
- `reasoning_tags` 是否正确配置
- 标签格式是否与实际输出匹配

### 2. 翻译结果被错误截断

可能原因：
- 推理标签配置错误
- 正文中包含类似推理标签的内容

解决方法：
- 检查实际输出格式
- 调整 `reasoning_tags` 配置
- 使用更精确的标签匹配

### 3. 性能问题

优化建议：
- 为短文本使用传统模型
- 设置合理的 `fast_mode_threshold`
- 使用缓存减少重复调用

## 未来展望

- 支持更多推理模型（Claude 3.5 Sonnet 等）
- 智能选择是否使用推理模型
- 推理过程的可选保存（用于分析）
- 基于推理深度的自动质量评分