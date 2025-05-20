package translator_tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 测试模拟OpenAI服务器的基本功能
func TestMockOpenAIServer(t *testing.T) {
	// 创建模拟服务器
	server := NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置默认响应
	server.SetDefaultResponse("这是测试响应")

	// 创建请求
	requestBody := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "测试请求",
			},
		},
		"temperature": 0.7,
	}
	requestJSON, _ := json.Marshal(requestBody)

	// 发送请求
	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewBuffer(requestJSON))
	assert.NoError(t, err)
	defer resp.Body.Close()

	// 验证响应状态码
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	// 解析响应
	var response map[string]interface{}
	err = json.Unmarshal(body, &response)
	assert.NoError(t, err)

	// 验证响应内容
	choices, ok := response["choices"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, choices, 1)

	choice, ok := choices[0].(map[string]interface{})
	assert.True(t, ok)

	message, ok := choice["message"].(map[string]interface{})
	assert.True(t, ok)

	content, ok := message["content"].(string)
	assert.True(t, ok)
	assert.Equal(t, "这是测试响应", content)

	// 验证请求日志
	logs := server.GetRequestLog()
	assert.Len(t, logs, 1)
	assert.Equal(t, "/v1/chat/completions", logs[0].Path)
	assert.Equal(t, "POST", logs[0].Method)
}

// 测试模拟OpenAI服务器的流式响应
func TestMockOpenAIServerStreaming(t *testing.T) {
	// 创建模拟服务器
	server := NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置默认响应和启用流式响应
	server.SetDefaultResponse("这是流式响应测试")
	server.SetStreamingEnabled(true)

	// 创建请求
	requestBody := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "流式测试请求",
			},
		},
		"temperature": 0.7,
		"stream":      true,
	}
	requestJSON, _ := json.Marshal(requestBody)

	// 发送请求
	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewBuffer(requestJSON))
	assert.NoError(t, err)
	defer resp.Body.Close()

	// 验证响应状态码
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 验证Content-Type
	contentType := resp.Header.Get("Content-Type")
	assert.Equal(t, "text/event-stream", contentType)

	// 读取流式响应
	reader := resp.Body
	buffer := make([]byte, 1024)
	var fullResponse string

	for {
		n, err := reader.Read(buffer)
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)

		chunk := string(buffer[:n])
		fullResponse += chunk

		// 如果收到[DONE]标记，表示流式响应结束
		if bytes.Contains(buffer[:n], []byte("data: [DONE]")) {
			break
		}
	}

	// 验证流式响应包含预期内容
	assert.Contains(t, fullResponse, "这是流式响应测试")
	assert.Contains(t, fullResponse, "data: [DONE]")

	// 验证请求日志
	logs := server.GetRequestLog()
	assert.Len(t, logs, 1)
	assert.Equal(t, "/v1/chat/completions", logs[0].Path)
	assert.Equal(t, "POST", logs[0].Method)
}

// 测试模拟OpenAI服务器的错误响应
func TestMockOpenAIServerError(t *testing.T) {
	// 创建模拟服务器
	server := NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置100%错误率
	server.SetErrorRate(1.0)

	// 创建请求
	requestBody := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "错误测试请求",
			},
		},
		"temperature": 0.7,
	}
	requestJSON, _ := json.Marshal(requestBody)

	// 发送请求
	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewBuffer(requestJSON))
	assert.NoError(t, err)
	defer resp.Body.Close()

	// 验证响应状态码
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	// 解析响应
	var response map[string]interface{}
	err = json.Unmarshal(body, &response)
	assert.NoError(t, err)

	// 验证错误响应
	errorObj, ok := response["error"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "模拟的API错误", errorObj["message"])
	assert.Equal(t, "api_error", errorObj["type"])
}

// 测试模拟OpenAI服务器的响应延迟
func TestMockOpenAIServerDelay(t *testing.T) {
	// 创建模拟服务器
	server := NewMockOpenAIServer(t)
	defer server.Stop()

	// 设置响应延迟
	delay := 500 * time.Millisecond
	server.SetResponseDelay(delay)

	// 创建请求
	requestBody := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "延迟测试请求",
			},
		},
		"temperature": 0.7,
	}
	requestJSON, _ := json.Marshal(requestBody)

	// 记录开始时间
	start := time.Now()

	// 发送请求
	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewBuffer(requestJSON))
	assert.NoError(t, err)
	defer resp.Body.Close()

	// 计算经过的时间
	elapsed := time.Since(start)

	// 验证响应延迟
	assert.GreaterOrEqual(t, elapsed, delay)
}
