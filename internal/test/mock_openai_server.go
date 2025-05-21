package test

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MockOpenAIServer 是一个模拟的OpenAI API服务器
type MockOpenAIServer struct {
	Server           *http.Server
	Port             int
	URL              string
	ResponseDelay    time.Duration
	DefaultResponse  string
	Responses        map[string]string // 根据请求内容返回不同的响应
	RequestLog       []MockRequest
	mu               sync.Mutex
	StreamingEnabled bool
	ErrorRate        float64 // 0-1之间，模拟错误率
}

// MockRequest 记录请求信息
type MockRequest struct {
	Path    string
	Method  string
	Headers map[string]string
	Body    map[string]interface{}
}

// NewMockOpenAIServer 创建一个新的模拟OpenAI服务器
func NewMockOpenAIServer(t interface{}) *MockOpenAIServer {
	// 找一个可用的随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("无法获取随机端口: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	server := &MockOpenAIServer{
		Port:             port,
		URL:              fmt.Sprintf("http://127.0.0.1:%d", port),
		ResponseDelay:    100 * time.Millisecond, // 默认延迟100ms
		DefaultResponse:  "这是一个模拟的OpenAI响应",
		Responses:        make(map[string]string),
		RequestLog:       make([]MockRequest, 0),
		StreamingEnabled: false,
		ErrorRate:        0.0,
	}

	// 创建HTTP服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", server.handleChatCompletions)
	mux.HandleFunc("/chat/completions", server.handleChatCompletions)

	server.Server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// 启动服务器
	if err := server.Start(); err != nil {
		log.Fatalf("无法启动模拟OpenAI服务器: %v", err)
	}

	return server
}

// Start 启动服务器
func (s *MockOpenAIServer) Start() error {
	go func() {
		if err := s.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("模拟OpenAI服务器错误: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)
	return nil
}

// Stop 停止服务器
func (s *MockOpenAIServer) Stop() error {
	return s.Server.Close()
}

// AddResponse 添加特定请求的响应
func (s *MockOpenAIServer) AddResponse(prompt string, response string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Responses[prompt] = response
}

// SetDefaultResponse 设置默认响应
func (s *MockOpenAIServer) SetDefaultResponse(response string) {
	s.DefaultResponse = response
}

// SetResponseDelay 设置响应延迟
func (s *MockOpenAIServer) SetResponseDelay(delay time.Duration) {
	s.ResponseDelay = delay
}

// SetStreamingEnabled 设置是否启用流式响应
func (s *MockOpenAIServer) SetStreamingEnabled(enabled bool) {
	s.StreamingEnabled = enabled
}

// SetErrorRate 设置错误率
func (s *MockOpenAIServer) SetErrorRate(rate float64) {
	s.ErrorRate = rate
}

// GetRequestLog 获取请求日志
func (s *MockOpenAIServer) GetRequestLog() []MockRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]MockRequest{}, s.RequestLog...)
}

// ClearRequestLog 清除请求日志
func (s *MockOpenAIServer) ClearRequestLog() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RequestLog = make([]MockRequest, 0)
}

// handleChatCompletions 处理聊天完成请求
func (s *MockOpenAIServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 记录请求
	var requestBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, fmt.Sprintf("无法解析请求体: %v", err), http.StatusBadRequest)
		return
	}

	headers := make(map[string]string)
	for k, v := range r.Header {
		headers[k] = strings.Join(v, ", ")
	}

	s.RequestLog = append(s.RequestLog, MockRequest{
		Path:    r.URL.Path,
		Method:  r.Method,
		Headers: headers,
		Body:    requestBody,
	})

	// 模拟处理延迟
	time.Sleep(s.ResponseDelay)

	// 随机模拟错误
	if rand.Float64() < s.ErrorRate {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "模拟的API错误",
				"type":    "api_error",
				"code":    "internal_error",
			},
		})
		return
	}

	// 获取请求中的消息
	var prompt string
	if messages, ok := requestBody["messages"].([]interface{}); ok && len(messages) > 0 {
		if message, ok := messages[0].(map[string]interface{}); ok {
			if content, ok := message["content"].(string); ok {
				prompt = content
			}
		}
	}

	// 确定响应内容
	responseContent := s.DefaultResponse
	if specificResponse, ok := s.Responses[prompt]; ok {
		responseContent = specificResponse
	}

	// 检查是否请求流式响应
	isStream := false
	if stream, ok := requestBody["stream"].(bool); ok {
		isStream = stream
	}

	if isStream && s.StreamingEnabled {
		// 发送流式响应
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// 将响应分成多个部分发送
		parts := strings.Split(responseContent, " ")
		for i, part := range parts {
			// 构建流式响应格式
			streamResponse := map[string]interface{}{
				"id":      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   "gpt-3.5-turbo",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"content": part + " ",
						},
						"finish_reason": nil,
					},
				},
			}

			// 最后一部分设置finish_reason
			if i == len(parts)-1 {
				streamResponse["choices"].([]map[string]interface{})[0]["finish_reason"] = "stop"
			}

			data, _ := json.Marshal(streamResponse)
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond) // 每个部分之间的延迟
		}

		// 发送结束标记
		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
	} else {
		// 发送普通JSON响应
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"id":      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-3.5-turbo",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": responseContent,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     len(prompt) / 4,
				"completion_tokens": len(responseContent) / 4,
				"total_tokens":      (len(prompt) + len(responseContent)) / 4,
			},
		}

		_ = json.NewEncoder(w).Encode(response)
	}
}
