package translator

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nerdneilsfield/go-translator-agent/internal/config"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

// statusRoundTripper 记录最近一次请求的状态码
type statusRoundTripper struct {
	base http.RoundTripper
	code *int
}

func (s *statusRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := s.base.RoundTrip(req)
	if err == nil && resp != nil && s.code != nil {
		*s.code = resp.StatusCode
	}
	return resp, err
}

// InitModels 初始化所有配置的语言模型
func InitModels(cfg *config.Config, log logger.Logger) (map[string]LLMClient, error) {
	models := make(map[string]LLMClient)

	// 设置超时时间
	timeout := 300 * time.Second // 默认 5 分钟
	if cfg.RequestTimeout > 0 {
		timeout = time.Duration(cfg.RequestTimeout) * time.Second
	}

	log.Info("InitModels called", zap.Int("model_configs_count", len(cfg.ModelConfigs)))

	// Debug: List all model names
	var modelNames []string
	for modelName := range cfg.ModelConfigs {
		modelNames = append(modelNames, modelName)
	}
	log.Info("All model names", zap.Strings("models", modelNames))

	for modelName, modelCfg := range cfg.ModelConfigs {
		var client LLMClient
		var err error

		log.Info("初始化模型客户端", zap.String("模型", modelName), zap.Any("配置", modelCfg.APIType))

		if modelCfg.APIType == "" {
			log.Error("模型配置缺少 APIType", zap.String("模型", modelName))
			return nil, fmt.Errorf("模型 %s 缺少 api_type 配置", modelName)
		}

		switch modelCfg.APIType {
		case "openai":
			// 使用标准OpenAI客户端
			client, err = newOpenAIClient(modelCfg, log, timeout)
		case "openai-reasoning":
			// 使用流式专用客户端（用于处理推理过程）
			client, err = newStreamOnlyOpenAIClient(modelCfg, log, timeout)
			log.Info("使用流式推理专用OpenAI客户端", zap.String("模型", modelName))
		case "raw":
			// 使用原始文本返回客户端
			client = NewRawClient()
			log.Info("使用原始文本返回客户端", zap.String("模型", modelName))
			err = nil
		case "anthropic":
			// 临时返回 OpenAI 客户端（未来完善 Anthropic 实现）
			log.Warn(fmt.Sprintf("Anthropic API 客户端尚未完全实现，使用 OpenAI 客户端代替: %s", modelName))
			client, err = newOpenAIClient(modelCfg, log, timeout)
		case "mistral":
			// 临时返回 OpenAI 客户端（未来完善 Mistral 实现）
			log.Warn(fmt.Sprintf("Mistral API 客户端尚未完全实现，使用 OpenAI 客户端代替: %s", modelName))
			client, err = newOpenAIClient(modelCfg, log, timeout)
		case "deepl", "deeplx", "google", "libretranslate":
			// 跳过专业翻译服务，这些不是 LLM
			log.Info("跳过专业翻译服务", zap.String("模型", modelName), zap.String("类型", modelCfg.APIType))
			continue
		default:
			return nil, fmt.Errorf("不支持的模型类型: %s", modelCfg.APIType)
		}

		if err != nil {
			return nil, fmt.Errorf("创建模型客户端失败 %s: %w", modelName, err)
		}

		models[modelName] = client
	}

	return models, nil
}

// OpenAIClient 是对OpenAI API的客户端封装
type OpenAIClient struct {
	client           *openai.Client
	modelName        string
	modelID          string // 用于 API 请求的模型 ID
	modelType        string
	maxInputTokens   int
	maxOutputTokens  int
	log              logger.Logger
	baseURL          string
	apiKey           string
	httpClient       *http.Client  // 用于自定义请求
	timeout          time.Duration // 请求超时时间
	inputTokenPrice  float64
	outputTokenPrice float64
	priceUnit        string
	lastStatusCode   int
}

// maskAuthToken 遮蔽认证令牌，只显示前4位和后4位
func maskAuthToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// newOpenAIClient 创建一个新的OpenAI客户端
func newOpenAIClient(cfg config.ModelConfig, log logger.Logger, timeout time.Duration) (*OpenAIClient, error) {
	var client *openai.Client
	var err error // Declare error variable for later use

	// 确定使用哪个模型 ID
	modelID := cfg.Name
	if cfg.ModelID != "" {
		modelID = cfg.ModelID
	}

	// 记录创建OpenAI客户端时使用的原始配置值
	maskedKey := maskAuthToken(cfg.Key)
	log.Debug("准备创建 OpenAI 客户端",
		zap.String("config_name", cfg.Name),
		zap.String("effective_model_id", modelID),
		zap.String("config_base_url", cfg.BaseURL),
		zap.String("config_api_key_masked", maskedKey),
		zap.Duration("timeout", timeout),
	)

	openAIClient := &OpenAIClient{
		modelName:        cfg.Name,
		modelID:          modelID,
		modelType:        "openai",
		maxInputTokens:   cfg.MaxInputTokens,
		maxOutputTokens:  cfg.MaxOutputTokens,
		log:              log,
		baseURL:          cfg.BaseURL, // Store original cfg.BaseURL for sendManualRequest and logging
		apiKey:           cfg.Key,
		timeout:          timeout,
		inputTokenPrice:  cfg.InputTokenPrice,
		outputTokenPrice: cfg.OutputTokenPrice,
		priceUnit:        cfg.PriceUnit,
	}

	// 创建自定义 HTTP 客户端并记录状态码
	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: &statusRoundTripper{base: http.DefaultTransport, code: &openAIClient.lastStatusCode},
	}
	openAIClient.httpClient = httpClient

	// 为 go-openai 库配置 openai.ClientConfig
	goOpenaiConfig := openai.DefaultConfig(cfg.Key)
	goOpenaiConfig.HTTPClient = httpClient

	if cfg.BaseURL != "" {
		// 如果配置了自定义 BaseURL
		// 确保传递给 go-openai 的 BaseURL 不以斜杠结尾，
		// 因为 go-openai 的 API 后缀通常以斜杠开头，避免出现双斜杠。
		goOpenaiConfig.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")

		log.Debug("为 go-openai 客户端配置了自定义 BaseURL",
			zap.String("original_cfg_base_url", cfg.BaseURL),
			zap.String("go_openai_target_base_url", goOpenaiConfig.BaseURL),
		)
		client = openai.NewClientWithConfig(goOpenaiConfig)
	} else {
		// 没有配置自定义 BaseURL，使用 OpenAI 的默认 URL (api.openai.com)
		// goOpenaiConfig 已包含 httpClient 和 API Key
		log.Debug("未配置 cfg.BaseURL，go-openai 客户端将使用默认 OpenAI API 地址")
		client = openai.NewClientWithConfig(goOpenaiConfig)
	}

	openAIClient.client = client // 将配置好的 go-openai 客户端实例赋给我们的包装器
	return openAIClient, err     // Return potential error from earlier stages if any
}

// GetInputTokenPrice 返回输入令牌价格
func (c *OpenAIClient) GetInputTokenPrice() float64 {
	return c.inputTokenPrice
}

// GetOutputTokenPrice 返回输出令牌价格
func (c *OpenAIClient) GetOutputTokenPrice() float64 {
	return c.outputTokenPrice
}

// GetPriceUnit 返回价格单位
func (c *OpenAIClient) GetPriceUnit() string {
	return c.priceUnit
}

// Complete 从提示词生成文本
func (c *OpenAIClient) Complete(prompt string, maxTokens int, temperature float64) (string, int, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	// 构建请求
	req := openai.ChatCompletionRequest{
		Model: c.modelID, // 使用模型 ID 而不是模型名称
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Temperature: float32(temperature),
		MaxTokens:   maxTokens,
	}

	// 记录请求详情
	maskedKey := maskAuthToken(c.apiKey)
	c.log.Debug("发送 API 请求",
		zap.String("模型", c.modelName),
		zap.String("模型ID", c.modelID),
		zap.String("基础URL", c.baseURL),
		zap.String("API密钥", maskedKey),
		zap.Float64("温度", temperature),
		zap.Int("最大令牌数", maxTokens),
		zap.Int("提示词长度", len(prompt)),
		zap.Duration("超时时间", c.timeout),
		zap.Float64("输入令牌价格", c.inputTokenPrice),
		zap.Float64("输出令牌价格", c.outputTokenPrice),
		zap.String("价格单位", c.priceUnit),
	)

	// 尝试使用标准客户端
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		// 详细记录错误信息，以便调试
		c.log.Error("API 调用失败",
			zap.String("模型", c.modelName),
			zap.String("模型ID", c.modelID),
			zap.String("基础URL", c.baseURL),
			zap.Error(err),
		)

		// 尝试手动发送请求以获取更多错误信息
		detailedError := c.sendManualRequest(ctx, req)

		// 如果 detailedError 为空，可能是因为虽然返回了非200状态码，但包含有效内容
		// 在这种情况下，我们尝试再次手动发送请求并解析内容
		if detailedError == "" {
			content, promptTokens, completionTokens := c.tryExtractContentFromErrorResponse(ctx, req)
			if content != "" {
				c.log.Info("API返回非200状态码但成功提取到有效内容",
					zap.Int("提示词令牌数", promptTokens),
					zap.Int("完成令牌数", completionTokens),
				)

				// 提取内容，根据配置决定是否过滤推理过程
				shouldFilter := false
				if cfg, ok := c.log.(interface{ GetConfig() *config.Config }); ok {
					shouldFilter = cfg.GetConfig().FilterReasoning
				}

				if shouldFilter {
					content = c.filterReasoningContent(content)
				}

				return content, promptTokens, completionTokens, nil
			}
		}

		if detailedError != "" {
			c.log.Error("OpenAI API调用失败",
				zap.String("详细错误", detailedError),
			)
			return "", 0, 0, fmt.Errorf("OpenAI API调用失败: %w\n详细错误: %s", err, detailedError)
		}

		return "", 0, 0, fmt.Errorf("OpenAI API调用失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		c.log.Error("API 返回空结果",
			zap.String("模型", c.modelName),
			zap.String("模型ID", c.modelID),
		)
		return "", 0, 0, fmt.Errorf("OpenAI返回空结果")
	}

	// 提取内容，根据配置决定是否过滤推理过程
	content := resp.Choices[0].Message.Content

	// 检查是否需要过滤推理过程
	shouldFilter := false
	if cfg, ok := c.log.(interface{ GetConfig() *config.Config }); ok {
		shouldFilter = cfg.GetConfig().FilterReasoning
	}

	if shouldFilter {
		content = c.filterReasoningContent(content)
		c.log.Debug("已过滤推理过程")
	}

	// 记录成功信息
	c.log.Debug("API 调用成功",
		zap.String("模型", c.modelName),
		zap.String("模型ID", c.modelID),
		zap.Int("状态码", c.lastStatusCode),
		zap.Int("提示词令牌数", resp.Usage.PromptTokens),
		zap.Int("完成令牌数", resp.Usage.CompletionTokens),
		zap.Int("总令牌数", resp.Usage.TotalTokens),
		zap.String("返回片段", snippet(content)),
	)

	// 返回结果
	return content, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, nil
}

// tryExtractContentFromErrorResponse 尝试从错误响应中提取有效内容
func (c *OpenAIClient) tryExtractContentFromErrorResponse(ctx context.Context, req openai.ChatCompletionRequest) (string, int, int) {
	// 构建请求 URL
	url := c.baseURL
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += "chat/completions"

	// 将请求转换为 JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		c.log.Error("序列化请求失败", zap.Error(err))
		return "", 0, 0
	}

	// 创建 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		c.log.Error("创建 HTTP 请求失败", zap.Error(err))
		return "", 0, 0
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	// 发送请求
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.log.Error("发送 HTTP 请求失败", zap.Error(err))
		return "", 0, 0
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.log.Error("读取响应失败", zap.Error(err))
		return "", 0, 0
	}

	// 尝试解析响应
	if len(respBody) > 0 {
		var jsonResponse map[string]interface{}
		if err := json.Unmarshal(respBody, &jsonResponse); err == nil {
			// 提取内容
			content := c.extractContentFromJSON(jsonResponse)
			if content != "" {
				// 提取令牌使用情况
				promptTokens, completionTokens := c.extractTokenUsageFromJSON(jsonResponse)
				c.log.Debug("成功从响应中提取内容和令牌使用情况",
					zap.Int("提示词令牌数", promptTokens),
					zap.Int("完成令牌数", completionTokens),
				)
				return content, promptTokens, completionTokens
			}
		}
	}

	return "", 0, 0
}

// extractContentFromJSON 从JSON响应中提取内容
func (c *OpenAIClient) extractContentFromJSON(jsonResponse map[string]interface{}) string {
	if choices, ok := jsonResponse["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			// 检查是否有 message 字段
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					return content
				}
			}
			// 检查是否有 delta 字段（流式响应）
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				if content, ok := delta["content"].(string); ok {
					return content
				}
			}
		}
	}
	return ""
}

// extractTokenUsageFromJSON 从JSON响应中提取令牌使用情况
func (c *OpenAIClient) extractTokenUsageFromJSON(jsonResponse map[string]interface{}) (int, int) {
	if usage, ok := jsonResponse["usage"].(map[string]interface{}); ok {
		promptTokens := 0
		completionTokens := 0

		if pt, ok := usage["prompt_tokens"].(float64); ok {
			promptTokens = int(pt)
		}
		if ct, ok := usage["completion_tokens"].(float64); ok {
			completionTokens = int(ct)
		}

		return promptTokens, completionTokens
	}
	return 0, 0
}

// CompleteStream 从提示词生成文本（流式响应）
func (c *OpenAIClient) CompleteStream(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	// 构建请求
	req := openai.ChatCompletionRequest{
		Model: c.modelID,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Temperature: float32(temperature),
		MaxTokens:   maxTokens,
		Stream:      true,
	}

	// 记录请求详情
	maskedKey := maskAuthToken(c.apiKey)
	c.log.Debug("发送流式 API 请求",
		zap.String("模型", c.modelName),
		zap.String("模型ID", c.modelID),
		zap.String("基础URL", c.baseURL),
		zap.String("API密钥", maskedKey),
		zap.Float64("温度", temperature),
		zap.Int("最大令牌数", maxTokens),
		zap.Int("提示词长度", len(prompt)),
	)

	// 创建流式响应
	stream, err := c.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		c.log.Error("创建流式响应失败", zap.Error(err))

		// 尝试手动发送流式请求
		content, streamErr := c.manualStreamRequest(ctx, req)
		if streamErr == nil && content != "" {
			c.log.Warn("通过手动流式请求成功获取内容",
				zap.Int("内容长度", len(content)),
			)
			return content, nil
		}

		return "", fmt.Errorf("创建流式响应失败: %w", err)
	}
	defer stream.Close()

	// 收集响应
	var fullResponse strings.Builder
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			c.log.Error("接收流式响应失败", zap.Error(err))

			// 如果已经收集了一些内容，可以考虑返回部分内容
			if fullResponse.Len() > 0 {
				c.log.Warn("流式响应中断，但已收集部分内容",
					zap.Int("已收集内容长度", fullResponse.Len()),
				)
				result := fullResponse.String()

				// 检查是否需要过滤推理过程
				shouldFilter := false
				if cfg, ok := c.log.(interface{ GetConfig() *config.Config }); ok {
					shouldFilter = cfg.GetConfig().FilterReasoning
				}

				if shouldFilter {
					result = c.filterReasoningContent(result)
				}

				return result, nil
			}

			return "", fmt.Errorf("接收流式响应失败: %w", err)
		}

		if len(response.Choices) > 0 {
			content := response.Choices[0].Delta.Content
			fullResponse.WriteString(content)
		}
	}

	// 获取完整响应
	result := fullResponse.String()

	// 检查是否需要过滤推理过程
	shouldFilter := false
	if cfg, ok := c.log.(interface{ GetConfig() *config.Config }); ok {
		shouldFilter = cfg.GetConfig().FilterReasoning
	}

	// 根据配置过滤推理内容
	if shouldFilter {
		result = c.filterReasoningContent(result)
		c.log.Debug("已过滤流式响应中的推理过程")
	}

	c.log.Debug("流式响应完成",
		zap.Int("响应长度", len(result)),
	)

	return result, nil
}

// manualStreamRequest 手动发送流式请求并处理响应
func (c *OpenAIClient) manualStreamRequest(ctx context.Context, req openai.ChatCompletionRequest) (string, error) {
	// 确保请求是流式的
	req.Stream = true

	// 构建请求 URL
	url := c.baseURL
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += "chat/completions"

	// 将请求转换为 JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		c.log.Error("序列化请求失败", zap.Error(err))
		return "", err
	}

	// 创建 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		c.log.Error("创建 HTTP 请求失败", zap.Error(err))
		return "", err
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	// 记录请求详情
	maskedKey := maskAuthToken(c.apiKey)
	c.log.Debug("手动发送流式请求",
		zap.String("URL", url),
		zap.String("API密钥", maskedKey),
	)

	// 发送请求
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.log.Error("发送 HTTP 请求失败", zap.Error(err))
		return "", err
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		// 即使状态码不是200，也尝试读取响应
		respBody, _ := io.ReadAll(resp.Body)
		c.log.Warn("流式请求返回非200状态码",
			zap.Int("状态码", resp.StatusCode),
			zap.String("响应体", string(respBody)),
		)

		// 尝试从错误响应中提取内容
		var jsonResponse map[string]interface{}
		if err := json.Unmarshal(respBody, &jsonResponse); err == nil {
			// 检查是否有错误信息
			if errorInfo, ok := jsonResponse["error"].(map[string]interface{}); ok {
				errorMessage := ""
				if msg, ok := errorInfo["message"].(string); ok {
					errorMessage = msg
				}
				errorType := ""
				if typ, ok := errorInfo["type"].(string); ok {
					errorType = typ
				}

				if errorMessage != "" || errorType != "" {
					c.log.Error("API返回错误信息",
						zap.String("错误类型", errorType),
						zap.String("错误消息", errorMessage),
					)
					return "", fmt.Errorf("API错误: %s - %s", errorType, errorMessage)
				}
			}

			// 尝试提取内容
			content := c.extractContentFromJSON(jsonResponse)
			if content != "" {
				c.log.Info("从非200响应中成功提取到有效内容",
					zap.Int("内容长度", len(content)),
				)
				return content, nil
			}
		}

		return "", fmt.Errorf("流式请求返回非200状态码: %d", resp.StatusCode)
	}

	// 读取SSE流
	scanner := bufio.NewScanner(resp.Body)
	var fullResponse strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// 处理数据行
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// 检查是否是流结束标记
			if data == "[DONE]" {
				break
			}

			// 解析JSON
			var streamResp map[string]interface{}
			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				c.log.Error("解析流式响应失败", zap.Error(err), zap.String("数据", data))
				continue
			}

			// 提取内容
			content := c.extractContentFromStreamJSON(streamResp)
			if content != "" {
				fullResponse.WriteString(content)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		c.log.Error("读取流式响应失败", zap.Error(err))

		// 如果已经收集了一些内容，返回部分内容
		if fullResponse.Len() > 0 {
			c.log.Warn("流式响应读取中断，但已收集部分内容",
				zap.Int("已收集内容长度", fullResponse.Len()),
			)
			return fullResponse.String(), nil
		}

		return "", err
	}

	return fullResponse.String(), nil
}

// extractContentFromStreamJSON 从流式JSON响应中提取内容
func (c *OpenAIClient) extractContentFromStreamJSON(jsonResponse map[string]interface{}) string {
	if choices, ok := jsonResponse["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			// 检查是否有 delta 字段
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				if content, ok := delta["content"].(string); ok {
					return content
				}
			}
		}
	}
	return ""
}

// filterReasoningContent 过滤掉推理过程内容
func (c *OpenAIClient) filterReasoningContent(content string) string {
	// 检查是否包含推理过程标记
	reasoningTags := []struct {
		start string
		end   string
	}{
		{start: "<reasoning>", end: "</reasoning>"},
		{start: "<Reasoning>", end: "</Reasoning>"},
		{start: "<REASONING>", end: "</REASONING>"},
		{start: "<思考>", end: "</思考>"},
		{start: "<思路>", end: "</思路>"},
		{start: "<推理>", end: "</推理>"},
		{start: "<分析>", end: "</分析>"},
		{start: "<Analysis>", end: "</Analysis>"},
		{start: "<ANALYSIS>", end: "</ANALYSIS>"},
		{start: "<think>", end: "</think>"},
		{start: "<Think>", end: "</Think>"},
		{start: "<THINK>", end: "</THINK>"},
	}

	// 检查是否包含任何推理标记
	hasReasoningTags := false
	for _, tag := range reasoningTags {
		if strings.Contains(content, tag.start) && strings.Contains(content, tag.end) {
			hasReasoningTags = true
			break
		}
	}

	if !hasReasoningTags {
		return content
	}

	// 移除所有推理部分
	result := content
	for _, tag := range reasoningTags {
		for {
			startIdx := strings.Index(result, tag.start)
			if startIdx == -1 {
				break
			}

			endIdx := strings.Index(result, tag.end)
			if endIdx == -1 {
				break
			}

			// 确保找到完整的标签对
			if endIdx > startIdx {
				// 移除推理部分
				result = result[:startIdx] + result[endIdx+len(tag.end):]
			} else {
				break
			}
		}
	}

	// 清理可能的多余空行
	result = strings.TrimSpace(result)

	// 记录过滤情况
	if result != content {
		c.log.Debug("已过滤推理内容",
			zap.Int("原始长度", len(content)),
			zap.Int("过滤后长度", len(result)),
		)
	}

	return result
}

// sendManualRequest 手动发送请求以获取更详细的错误信息
func (c *OpenAIClient) sendManualRequest(ctx context.Context, req openai.ChatCompletionRequest) string {
	// 构建请求 URL
	url := c.baseURL
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += "chat/completions"

	// 将请求转换为 JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		c.log.Error("序列化请求失败", zap.Error(err))
		return ""
	}

	// 创建 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		c.log.Error("创建 HTTP 请求失败", zap.Error(err))
		return ""
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	// 记录请求详情（使用遮蔽的令牌）
	maskedKey := maskAuthToken(c.apiKey)
	c.log.Debug("手动 HTTP 请求详情",
		zap.String("URL", url),
		zap.String("方法", "POST"),
		zap.String("API密钥", maskedKey),
		zap.Int("请求体长度", len(reqBody)),
	)

	// 发送请求
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.log.Error("发送 HTTP 请求失败", zap.Error(err))
		return ""
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.log.Error("读取响应失败", zap.Error(err))
		return ""
	}

	// 记录响应详情
	c.log.Debug("HTTP 响应详情",
		zap.Int("状态码", resp.StatusCode),
		zap.Int("响应体长度", len(respBody)),
	)

	// 检查响应是否包含有效的JSON内容
	if len(respBody) > 0 {
		var jsonResponse map[string]interface{}
		if err := json.Unmarshal(respBody, &jsonResponse); err == nil {
			// 检查是否有错误信息
			if errorInfo, ok := jsonResponse["error"].(map[string]interface{}); ok {
				errorMessage := ""
				if msg, ok := errorInfo["message"].(string); ok {
					errorMessage = msg
				}
				errorType := ""
				if typ, ok := errorInfo["type"].(string); ok {
					errorType = typ
				}

				if errorMessage != "" || errorType != "" {
					c.log.Error("API返回错误信息",
						zap.String("错误类型", errorType),
						zap.String("错误消息", errorMessage),
					)
					return fmt.Sprintf("错误类型: %s, 错误消息: %s", errorType, errorMessage)
				}
			}

			// 如果能解析为JSON，检查是否包含有效的翻译结果
			if choices, ok := jsonResponse["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if message, ok := choice["message"].(map[string]interface{}); ok {
						if content, ok := message["content"].(string); ok && content != "" {
							// 发现有效的翻译内容，记录警告但不视为错误
							c.log.Info("手动请求成功并提取到内容",
								zap.Int("状态码", resp.StatusCode),
								zap.Int("内容长度", len(content)),
							)
							// 返回空字符串，表示不视为错误
							return ""
						}
					}
				}
			}
		}
	}

	return string(respBody)
}

// Name 返回模型名称
func (c *OpenAIClient) Name() string {
	return c.modelName
}

// Type 返回模型类型
func (c *OpenAIClient) Type() string {
	return c.modelType
}

// MaxInputTokens 返回模型支持的最大输入令牌数
func (c *OpenAIClient) MaxInputTokens() int {
	return c.maxInputTokens
}

// MaxOutputTokens 返回模型支持的最大输出令牌数
func (c *OpenAIClient) MaxOutputTokens() int {
	return c.maxOutputTokens
}

// StreamOnlyOpenAIClient 是对只支持流式模式的OpenAI兼容API的客户端封装
type StreamOnlyOpenAIClient struct {
	OpenAIClient // 继承 OpenAIClient 的大部分功能
}

// newStreamOnlyOpenAIClient 创建一个新的只支持流式模式的OpenAI客户端
func newStreamOnlyOpenAIClient(cfg config.ModelConfig, log logger.Logger, timeout time.Duration) (*StreamOnlyOpenAIClient, error) {
	baseClient, err := newOpenAIClient(cfg, log, timeout)
	if err != nil {
		return nil, err
	}

	log.Info("创建流式专用OpenAI客户端",
		zap.String("模型", cfg.Name),
		zap.String("模型ID", cfg.ModelID),
		zap.String("基础URL", cfg.BaseURL),
	)

	return &StreamOnlyOpenAIClient{
		OpenAIClient: *baseClient,
	}, nil
}

// Complete 重写Complete方法，使用流式API
func (c *StreamOnlyOpenAIClient) Complete(prompt string, maxTokens int, temperature float64) (string, int, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	// 记录请求详情
	maskedKey := maskAuthToken(c.apiKey)
	c.log.Debug("发送流式API请求",
		zap.String("模型", c.modelName),
		zap.String("模型ID", c.modelID),
		zap.String("基础URL", c.baseURL),
		zap.String("API密钥", maskedKey),
		zap.Float64("温度", temperature),
		zap.Int("最大令牌数", maxTokens),
		zap.Int("提示词长度", len(prompt)),
		zap.Float64("输入令牌价格", c.inputTokenPrice),
		zap.Float64("输出令牌价格", c.outputTokenPrice),
		zap.String("价格单位", c.priceUnit),
	)

	// 使用流式API
	content, err := c.CompleteStream(ctx, prompt, maxTokens, temperature)
	if err != nil {
		return "", 0, 0, err
	}

	// 由于流式API无法获取准确的令牌计数，我们使用估计值
	// 假设每个字符约为0.3个令牌（中文可能更高）
	promptTokens := int(float64(len(prompt)) * 0.3)
	completionTokens := int(float64(len(content)) * 0.3)

	c.log.Debug("流式API调用成功",
		zap.Int("估计提示词令牌数", promptTokens),
		zap.Int("估计完成令牌数", completionTokens),
		zap.Int("响应长度", len(content)),
	)

	return content, promptTokens, completionTokens, nil
}

/*
// 注释掉未实现的客户端代码，待将来实现
// 这些代码仅作为框架，将来会根据实际SDK实现进行具体编写

// AnthropicClient 是对Anthropic API的客户端封装
type AnthropicClient struct {
	client        any // *anthropic.Client
	modelName     string
	modelType     string
	maxInputTokens  int
	maxOutputTokens int
	log           logger.Logger
}

// MistralClient 是对Mistral AI API的客户端封装
type MistralClient struct {
	client        any // *mistralai.Client
	modelName     string
	modelType     string
	maxInputTokens  int
	maxOutputTokens int
	log           logger.Logger
}
*/
