package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"
)

// MockOpenAIResponse 模拟OpenAI API的响应
type MockOpenAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// MockOpenAIStreamResponse 模拟OpenAI API的流式响应
type MockOpenAIStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
	} `json:"choices"`
}

// CreateMockOpenAIServer 创建一个模拟的OpenAI API服务器
func CreateMockOpenAIServer(responseText string, errorRate float64, delayMs int) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模拟延迟
		if delayMs > 0 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}

		// 模拟错误
		if errorRate > 0 && errorRate > float64(time.Now().UnixNano()%100)/100.0 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": {"message": "模拟服务器错误", "type": "server_error"}}`))
			return
		}

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
			if len(responseText) <= 10 {
				chunks = append(chunks, responseText)
			} else {
				for i := 0; i < len(responseText); i += 10 {
					end := i + 10
					if end > len(responseText) {
						end = len(responseText)
					}
					chunks = append(chunks, responseText[i:end])
				}
			}

			// 发送开始消息
			startResp := MockOpenAIStreamResponse{
				ID:      "chatcmpl-mock",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   "gpt-3.5-turbo",
				Choices: []struct {
					Delta struct {
						Role    string `json:"role,omitempty"`
						Content string `json:"content,omitempty"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
					Index        int    `json:"index"`
				}{
					{
						Delta: struct {
							Role    string `json:"role,omitempty"`
							Content string `json:"content,omitempty"`
						}{
							Role: "assistant",
						},
						FinishReason: "",
						Index:        0,
					},
				},
			}
			startRespBytes, _ := json.Marshal(startResp)
			fmt.Fprintf(w, "data: %s\n\n", startRespBytes)
			w.(http.Flusher).Flush()

			// 发送内容块
			for _, chunk := range chunks {
				chunkResp := MockOpenAIStreamResponse{
					ID:      "chatcmpl-mock",
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   "gpt-3.5-turbo",
					Choices: []struct {
						Delta struct {
							Role    string `json:"role,omitempty"`
							Content string `json:"content,omitempty"`
						} `json:"delta"`
						FinishReason string `json:"finish_reason"`
						Index        int    `json:"index"`
					}{
						{
							Delta: struct {
								Role    string `json:"role,omitempty"`
								Content string `json:"content,omitempty"`
							}{
								Content: chunk,
							},
							FinishReason: "",
							Index:        0,
						},
					},
				}
				chunkRespBytes, _ := json.Marshal(chunkResp)
				fmt.Fprintf(w, "data: %s\n\n", chunkRespBytes)
				w.(http.Flusher).Flush()
				time.Sleep(50 * time.Millisecond) // 模拟网络延迟
			}

			// 发送结束消息
			endResp := MockOpenAIStreamResponse{
				ID:      "chatcmpl-mock",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   "gpt-3.5-turbo",
				Choices: []struct {
					Delta struct {
						Role    string `json:"role,omitempty"`
						Content string `json:"content,omitempty"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
					Index        int    `json:"index"`
				}{
					{
						Delta: struct {
							Role    string `json:"role,omitempty"`
							Content string `json:"content,omitempty"`
						}{},
						FinishReason: "stop",
						Index:        0,
					},
				},
			}
			endRespBytes, _ := json.Marshal(endResp)
			fmt.Fprintf(w, "data: %s\n\n", endRespBytes)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			w.(http.Flusher).Flush()
		} else {
			// 处理普通请求
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			resp := MockOpenAIResponse{
				ID:      "chatcmpl-mock",
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   "gpt-3.5-turbo",
				Choices: []struct {
					Message struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"message"`
					FinishReason string `json:"finish_reason"`
					Index        int    `json:"index"`
				}{
					{
						Message: struct {
							Role    string `json:"role"`
							Content string `json:"content"`
						}{
							Role:    "assistant",
							Content: responseText,
						},
						FinishReason: "stop",
						Index:        0,
					},
				},
				Usage: struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				}{
					PromptTokens:     100,
					CompletionTokens: 50,
					TotalTokens:      150,
				},
			}

			json.NewEncoder(w).Encode(resp)
		}
	}))

	return server
}
