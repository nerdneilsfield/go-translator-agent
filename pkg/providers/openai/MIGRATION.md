# OpenAI Provider 迁移指南

本文档说明如何从自定义实现迁移到使用官方 OpenAI Go SDK 的版本。

## 为什么迁移？

使用官方 SDK 的优势：
- 🔄 自动更新和维护
- 🛡️ 更好的错误处理和重试机制
- 🚀 性能优化
- 📡 流式响应支持
- 🔧 更丰富的配置选项
- 📚 官方文档支持

## 快速迁移

### 1. 更新导入

旧版本：
```go
provider := openai.New(config)
```

新版本：
```go
provider := openai.NewV2(config)
```

### 2. 配置更新

旧版本：
```go
config := openai.DefaultConfig()
config.APIKey = "your-key"
```

新版本：
```go
config := openai.DefaultConfigV2()
config.APIKey = "your-key"
config.OrgID = "your-org-id" // 可选
```

### 3. LLMClient 使用

旧版本：
```go
client := openai.NewLLMClient(config)
```

新版本：
```go
client := openai.NewLLMClientV2(config)
```

## 新功能

### 流式响应

新版本支持流式响应，适合实时翻译场景：

```go
provider := openai.NewV2(config)

chunks, err := provider.StreamTranslate(ctx, req)
if err != nil {
    return err
}

for chunk := range chunks {
    if chunk.Error != nil {
        return chunk.Error
    }
    fmt.Print(chunk.Text) // 实时输出翻译内容
}
```

### 更灵活的配置

```go
config := openai.ConfigV2{
    BaseConfig: providers.BaseConfig{
        APIKey:     "your-key",
        APIEndpoint: "https://your-proxy.com/v1", // 自定义端点
        Timeout:    60 * time.Second,
        MaxRetries: 5,
        Headers: map[string]string{
            "X-Custom-Header": "value",
        },
    },
    Model:       "gpt-4-turbo-preview",
    Temperature: 0.2,
    MaxTokens:   4096,
    OrgID:       "org-xxx", // 组织ID
}
```

### 自动重试和超时

官方 SDK 自动处理：
- 速率限制（429错误）
- 临时网络错误
- 服务器错误（5xx）

## 兼容性

### 向后兼容

新版本保持了相同的接口，可以直接替换：

```go
// 两个版本都实现了相同的接口
var provider translation.TranslationProvider
provider = openai.New(config)    // 旧版本
provider = openai.NewV2(config)   // 新版本
```

### 功能对比

| 功能 | 旧版本 | 新版本 (V2) |
|------|--------|-------------|
| 基本翻译 | ✅ | ✅ |
| 三步翻译 | ✅ | ✅ |
| 错误重试 | 手动实现 | 自动 |
| 流式响应 | ❌ | ✅ |
| 代理支持 | 需要自定义 | 内置 |
| 组织ID | ❌ | ✅ |
| 速率限制处理 | 基础 | 高级 |

## 迁移步骤

### 步骤 1：更新依赖

```bash
go get github.com/openai/openai-go@latest
```

### 步骤 2：更新代码

查找并替换：
```bash
# 查找使用旧版本的文件
grep -r "openai.New(" . | grep -v "NewV2"

# 查找使用旧LLMClient的文件
grep -r "openai.NewLLMClient(" . | grep -v "NewLLMClientV2"
```

### 步骤 3：测试

运行测试确保功能正常：
```bash
go test ./... -v
```

### 步骤 4：性能测试

新版本应该有更好的性能：
```bash
go test -bench=. ./pkg/providers/openai
```

## 故障排除

### 常见问题

1. **代理设置**
   ```go
   config.APIEndpoint = "https://your-proxy.com/v1"
   ```

2. **超时调整**
   ```go
   config.Timeout = 2 * time.Minute // 长文本翻译
   ```

3. **模型选择**
   ```go
   config.Model = "gpt-4-1106-preview" // 最新模型
   ```

### 错误处理

新版本提供更详细的错误信息：

```go
resp, err := provider.Translate(ctx, req)
if err != nil {
    // 官方SDK提供结构化错误
    // 可以检查具体错误类型和代码
    log.Printf("Translation failed: %v", err)
}
```

## 最佳实践

1. **使用上下文控制超时**
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **配置重用**
   ```go
   // 创建一次，多处使用
   var defaultProvider = openai.NewV2(myConfig)
   ```

3. **错误处理**
   ```go
   if err != nil {
       // 记录详细错误信息
       log.Printf("Provider: %s, Error: %v", provider.GetName(), err)
   }
   ```

## 需要帮助？

- 查看[官方SDK文档](https://github.com/openai/openai-go)
- 查看[示例代码](./examples/openai_official/main.go)
- 提交 Issue 到项目仓库