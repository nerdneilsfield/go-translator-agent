package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// MockOpenAIServer 是一个模拟的OpenAI API服务器
type MockOpenAIServer struct {
	Server          *httptest.Server
	URL             string
	Responses       map[string]string
	DefaultResponse string
	ErrorRate       float64
	DelayMs         int
	mu              sync.Mutex
}

// MockRequest 记录请求信息

// NewMockOpenAIServer 创建一个新的模拟OpenAI服务器
func NewMockOpenAIServer(t *testing.T) *MockOpenAIServer { //nolint:revive // Parameter t restored, revive lint ignored for now
	mock := &MockOpenAIServer{
		Responses:       make(map[string]string),
		DefaultResponse: "这是翻译后的文本",
		ErrorRate:       0.0,
		DelayMs:         0,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 解析请求体
		var requestBody struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}

		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&requestBody); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": {"message": "无法解析请求体", "type": "invalid_request_error"}}`))
			return
		}

		// 模拟延迟
		if mock.DelayMs > 0 {
			time.Sleep(time.Duration(mock.DelayMs) * time.Millisecond)
		}

		// 模拟错误
		mock.mu.Lock()
		errorRate := mock.ErrorRate
		mock.mu.Unlock()
		if errorRate > 0 && errorRate > float64(time.Now().UnixNano()%100)/100.0 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": {"message": "模拟服务器错误", "type": "server_error"}}`))
			return
		}

		// 获取用户消息
		var userMessage string
		for _, msg := range requestBody.Messages {
			if msg.Role == "user" {
				userMessage = msg.Content
				break
			}
		}

		// 查找响应
		mock.mu.Lock()
		response, ok := mock.Responses[userMessage]
		if !ok {
			response = mock.DefaultResponse
		}
		mock.mu.Unlock()

		// 检查是否为流式请求
		isStream := false
		for _, v := range r.Header.Values("Accept") {
			if v == "text/event-stream" {
				isStream = true
				break
			}
		}

		if isStream {
			// 处理流式请求
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)

			// 将响应文本分成多个部分发送
			chunks := []string{}
			if len(response) <= 10 {
				chunks = append(chunks, response)
			} else {
				for i := 0; i < len(response); i += 10 {
					end := i + 10
					if end > len(response) {
						end = len(response)
					}
					chunks = append(chunks, response[i:end])
				}
			}

			// 发送开始消息
			fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":%d,\"model\":\"%s\",\"choices\":[{\"delta\":{\"role\":\"assistant\"},\"index\":0,\"finish_reason\":null}]}\n\n", time.Now().Unix(), requestBody.Model)
			w.(http.Flusher).Flush()

			// 发送内容块
			for _, chunk := range chunks {
				fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":%d,\"model\":\"%s\",\"choices\":[{\"delta\":{\"content\":\"%s\"},\"index\":0,\"finish_reason\":null}]}\n\n", time.Now().Unix(), requestBody.Model, chunk)
				w.(http.Flusher).Flush()
				time.Sleep(50 * time.Millisecond) // 模拟网络延迟
			}

			// 发送结束消息
			fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.completion.chunk\",\"created\":%d,\"model\":\"%s\",\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"stop\"}]}\n\n", time.Now().Unix(), requestBody.Model)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			w.(http.Flusher).Flush()
		} else {
			// 处理普通请求
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// 构建响应
			responseBody := map[string]interface{}{
				"id":      "chatcmpl-mock",
				"object":  "chat.completion",
				"created": time.Now().Unix(),
				"model":   requestBody.Model,
				"choices": []map[string]interface{}{
					{
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": response,
						},
						"finish_reason": "stop",
						"index":         0,
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     100,
					"completion_tokens": 50,
					"total_tokens":      150,
				},
			}

			_ = json.NewEncoder(w).Encode(responseBody)
		}
	}))

	mock.Server = server
	mock.URL = server.URL

	// 添加清理函数
	t.Cleanup(func() {
		server.Close()
	})

	return mock
}

// AddResponse 添加特定的请求-响应对
func (m *MockOpenAIServer) AddResponse(request, response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses[request] = response
}

// SetDefaultResponse 设置默认响应
func (m *MockOpenAIServer) SetDefaultResponse(response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DefaultResponse = response
}

// SetErrorRate 设置错误率
func (m *MockOpenAIServer) SetErrorRate(rate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrorRate = rate
}

// SetDelay 设置延迟时间（毫秒）
func (m *MockOpenAIServer) SetDelay(delayMs int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DelayMs = delayMs
}

// Stop 停止服务器
func (m *MockOpenAIServer) Stop() {
	m.Server.Close()
}
