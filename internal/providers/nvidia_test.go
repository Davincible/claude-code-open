package providers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNvidiaProvider_BasicMethods(t *testing.T) {
	provider := NewNvidiaProvider()

	assert.Equal(t, "nvidia", provider.Name())
	assert.True(t, provider.SupportsStreaming())

	provider.SetAPIKey("test-key")
	assert.Equal(t, "test-key", provider.apiKey)
}

func TestNvidiaProvider_IsStreaming(t *testing.T) {
	provider := NewNvidiaProvider()

	tests := []struct {
		name     string
		headers  map[string][]string
		expected bool
	}{
		{
			name: "content-type event-stream",
			headers: map[string][]string{
				"Content-Type": {"text/event-stream"},
			},
			expected: true,
		},
		{
			name: "transfer-encoding chunked",
			headers: map[string][]string{
				"Transfer-Encoding": {"chunked"},
			},
			expected: true,
		},
		{
			name: "no streaming headers",
			headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.IsStreaming(tt.headers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNvidiaProvider_TransformRequest(t *testing.T) {
	provider := NewNvidiaProvider()

	// Test Anthropic to OpenAI/Nvidia request transformation
	anthropicRequest := map[string]any{
		"model":      "claude-3-5-sonnet",
		"system":     "You are a helpful assistant",
		"max_tokens": 100,
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello, world!",
			},
		},
		"tools": []any{
			map[string]any{
				"name":        "get_weather",
				"description": "Get current weather",
				"input_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "City name",
						},
					},
					"required": []string{"location"},
				},
			},
		},
		"tool_choice": "auto",
	}

	anthropicJSON, err := json.Marshal(anthropicRequest)
	require.NoError(t, err)

	result, err := provider.TransformRequest(anthropicJSON)
	require.NoError(t, err)

	var nvidiaReq map[string]any
	err = json.Unmarshal(result, &nvidiaReq)
	require.NoError(t, err)

	// Verify system message was moved to messages array (OpenAI format)
	assert.NotContains(t, nvidiaReq, "system", "system field should be removed from root")
	messages, ok := nvidiaReq["messages"].([]any)
	require.True(t, ok, "messages should be an array")
	require.Len(t, messages, 2, "should have system + user message")

	systemMsg := messages[0].(map[string]any)
	assert.Equal(t, "system", systemMsg["role"])
	assert.Equal(t, "You are a helpful assistant", systemMsg["content"])

	// Verify max_tokens -> max_completion_tokens transformation
	assert.NotContains(t, nvidiaReq, "max_tokens", "max_tokens should be converted")
	assert.Equal(t, float64(100), nvidiaReq["max_completion_tokens"], "should have max_completion_tokens")

	// Verify tools transformation to OpenAI format
	tools, ok := nvidiaReq["tools"].([]any)
	require.True(t, ok, "tools should be an array")
	require.Len(t, tools, 1, "should have one tool")

	tool := tools[0].(map[string]any)
	assert.Equal(t, "function", tool["type"])
	function := tool["function"].(map[string]any)
	assert.Equal(t, "get_weather", function["name"])
	assert.Contains(t, function, "parameters", "should have parameters not input_schema")

	// Verify tool_choice is preserved
	assert.Equal(t, "auto", nvidiaReq["tool_choice"])
}

func TestNvidiaProvider_Transform(t *testing.T) {
	provider := NewNvidiaProvider()

	nvidiaResponse := map[string]any{
		"id":      "chatcmpl-nvidia-123",
		"object":  "chat.completion",
		"created": 1677652288,
		"model":   "nvidia/llama-3.1-nemotron-70b-instruct",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "Hello! How can I help you today?",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     9,
			"completion_tokens": 12,
			"total_tokens":      21,
		},
	}

	nvidiaJSON, err := json.Marshal(nvidiaResponse)
	require.NoError(t, err)

	result, err := provider.TransformResponse(nvidiaJSON)
	require.NoError(t, err)

	var anthropicResp map[string]any
	err = json.Unmarshal(result, &anthropicResp)
	require.NoError(t, err)

	// Check basic structure
	assert.Equal(t, "chatcmpl-nvidia-123", anthropicResp["id"])
	assert.Equal(t, "message", anthropicResp["type"])
	assert.Equal(t, "assistant", anthropicResp["role"])
	assert.Equal(t, "nvidia/llama-3.1-nemotron-70b-instruct", anthropicResp["model"])

	// Check content
	content, ok := anthropicResp["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)

	textBlock := content[0].(map[string]any)
	assert.Equal(t, "text", textBlock["type"])
	text, ok := textBlock["text"]
	require.True(t, ok)
	if textPtr, isPtr := text.(*string); isPtr {
		assert.Equal(t, "Hello! How can I help you today?", *textPtr)
	} else {
		assert.Equal(t, "Hello! How can I help you today?", text.(string))
	}

	// Check usage
	usage, ok := anthropicResp["usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(9), usage["input_tokens"])
	assert.Equal(t, float64(12), usage["output_tokens"])

	// Check stop reason
	stopReason, ok := anthropicResp["stop_reason"]
	require.True(t, ok)
	if stopPtr, isPtr := stopReason.(*string); isPtr {
		assert.Equal(t, "end_turn", *stopPtr)
	} else {
		assert.Equal(t, "end_turn", stopReason.(string))
	}
}

func TestNvidiaProvider_ConvertStopReason(t *testing.T) {
	provider := NewNvidiaProvider()

	tests := []struct {
		nvidiaReason      string
		expectedAnthropic string
	}{
		{"stop", "end_turn"},
		{"length", "max_tokens"},
		{"tool_calls", "tool_use"},
		{"function_call", "tool_use"},
		{"content_filter", "stop_sequence"},
		{"null", "end_turn"},
		{"unknown", "end_turn"},
	}

	for _, tt := range tests {
		t.Run(tt.nvidiaReason, func(t *testing.T) {
			result := provider.convertStopReason(tt.nvidiaReason)
			assert.Equal(t, tt.expectedAnthropic, *result)
		})
	}
}

func TestNvidiaProvider_ToolCallsTransform(t *testing.T) {
	provider := NewNvidiaProvider()

	nvidiaResponse := map[string]any{
		"id":      "chatcmpl-nvidia-123",
		"object":  "chat.completion",
		"created": 1677652288,
		"model":   "nvidia/llama-3.1-nemotron-70b-instruct",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []map[string]any{
						{
							"id":   "call_nvidia123",
							"type": "function",
							"function": map[string]any{
								"name":      "get_weather",
								"arguments": "{\"location\":\"San Francisco\",\"unit\":\"celsius\"}",
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     9,
			"completion_tokens": 12,
			"total_tokens":      21,
		},
	}

	nvidiaJSON, err := json.Marshal(nvidiaResponse)
	require.NoError(t, err)

	result, err := provider.TransformResponse(nvidiaJSON)
	require.NoError(t, err)

	var anthropicResp map[string]any
	err = json.Unmarshal(result, &anthropicResp)
	require.NoError(t, err)

	// Check content contains tool use
	content, ok := anthropicResp["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)

	toolBlock := content[0].(map[string]any)
	assert.Equal(t, "tool_use", toolBlock["type"])

	id, ok := toolBlock["id"]
	require.True(t, ok)
	if idPtr, isPtr := id.(*string); isPtr {
		assert.Equal(t, "toolu_nvidia123", *idPtr)
	} else {
		assert.Equal(t, "toolu_nvidia123", id.(string))
	}

	name, ok := toolBlock["name"]
	require.True(t, ok)
	if namePtr, isPtr := name.(*string); isPtr {
		assert.Equal(t, "get_weather", *namePtr)
	} else {
		assert.Equal(t, "get_weather", name.(string))
	}

	// Check tool input
	input, ok := toolBlock["input"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "San Francisco", input["location"])
	assert.Equal(t, "celsius", input["unit"])

	// Check stop reason
	stopReason, ok := anthropicResp["stop_reason"]
	require.True(t, ok)
	if stopPtr, isPtr := stopReason.(*string); isPtr {
		assert.Equal(t, "tool_use", *stopPtr)
	} else {
		assert.Equal(t, "tool_use", stopReason.(string))
	}
}

func TestNvidiaProvider_ErrorHandling(t *testing.T) {
	provider := NewNvidiaProvider()

	errorResponse := map[string]any{
		"error": map[string]any{
			"message": "Invalid API key",
			"type":    "authentication_error",
			"code":    "invalid_api_key",
		},
	}

	errorJSON, err := json.Marshal(errorResponse)
	require.NoError(t, err)

	result, err := provider.TransformResponse(errorJSON)
	require.NoError(t, err)

	var anthropicResp map[string]any
	err = json.Unmarshal(result, &anthropicResp)
	require.NoError(t, err)

	assert.Equal(t, "error", anthropicResp["type"])

	errorInfo, ok := anthropicResp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "authentication_error", errorInfo["type"])
	assert.Equal(t, "Invalid API key", errorInfo["message"])
}

func TestNvidiaProvider_TransformStream(t *testing.T) {
	provider := NewNvidiaProvider()
	state := &StreamState{}

	// Test message start chunk
	messageStartChunk := map[string]any{
		"id":    "chatcmpl-nvidia-123",
		"model": "nvidia/llama-3.1-nemotron-70b-instruct",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"role": "assistant",
				},
			},
		},
	}

	chunkJSON, err := json.Marshal(messageStartChunk)
	require.NoError(t, err)

	events, err := provider.TransformStream(chunkJSON, state)
	require.NoError(t, err)

	// Should generate message_start event
	eventStr := string(events)
	assert.Contains(t, eventStr, "event: message_start")
	assert.Contains(t, eventStr, "chatcmpl-nvidia-123")
	assert.True(t, state.MessageStartSent)

	// Test text content chunk
	textChunk := map[string]any{
		"id":    "chatcmpl-nvidia-123",
		"model": "nvidia/llama-3.1-nemotron-70b-instruct",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"content": "Hello!",
				},
			},
		},
	}

	chunkJSON, err = json.Marshal(textChunk)
	require.NoError(t, err)

	events, err = provider.TransformStream(chunkJSON, state)
	require.NoError(t, err)

	eventStr = string(events)
	assert.Contains(t, eventStr, "event: content_block_start")
	assert.Contains(t, eventStr, "event: content_block_delta")
	assert.Contains(t, eventStr, "Hello!")

	// Test finish chunk
	finishChunk := map[string]any{
		"id":    "chatcmpl-nvidia-123",
		"model": "nvidia/llama-3.1-nemotron-70b-instruct",
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"completion_tokens": 5,
		},
	}

	chunkJSON, err = json.Marshal(finishChunk)
	require.NoError(t, err)

	events, err = provider.TransformStream(chunkJSON, state)
	require.NoError(t, err)

	eventStr = string(events)
	assert.Contains(t, eventStr, "event: content_block_stop")
	assert.Contains(t, eventStr, "event: message_delta")
	assert.Contains(t, eventStr, "event: message_stop")
	assert.Contains(t, eventStr, "end_turn")
}

func TestNvidiaProvider_StreamingToolCalls(t *testing.T) {
	provider := NewNvidiaProvider()
	state := &StreamState{}

	// First chunk with tool call start
	toolCallStartChunk := map[string]any{
		"id":    "chatcmpl-nvidia-123",
		"model": "nvidia/llama-3.1-nemotron-70b-instruct",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"tool_calls": []map[string]any{
						{
							"index": 0,
							"id":    "call_nvidia123",
							"type":  "function",
							"function": map[string]any{
								"name":      "ls",
								"arguments": "",
							},
						},
					},
				},
			},
		},
	}

	chunkJSON, err := json.Marshal(toolCallStartChunk)
	require.NoError(t, err)

	events, err := provider.TransformStream(chunkJSON, state)
	require.NoError(t, err)

	eventStr := string(events)
	assert.Contains(t, eventStr, "event: content_block_start")
	assert.Contains(t, eventStr, "toolu_nvidia123")
	assert.Contains(t, eventStr, "tool_use")

	// Second chunk with arguments
	toolCallArgsChunk := map[string]any{
		"id":    "chatcmpl-nvidia-123",
		"model": "nvidia/llama-3.1-nemotron-70b-instruct",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"tool_calls": []map[string]any{
						{
							"index": 0,
							"function": map[string]any{
								"arguments": "{\"path\":\"/home\"}",
							},
						},
					},
				},
			},
		},
	}

	chunkJSON, err = json.Marshal(toolCallArgsChunk)
	require.NoError(t, err)

	events, err = provider.TransformStream(chunkJSON, state)
	require.NoError(t, err)

	eventStr = string(events)
	assert.Contains(t, eventStr, "event: content_block_delta")
	assert.Contains(t, eventStr, "input_json_delta")
	assert.Contains(t, eventStr, "/home")
}

func TestNvidiaProvider_ConvertUsage(t *testing.T) {
	provider := NewNvidiaProvider()

	usage := map[string]any{
		"prompt_tokens":     100,
		"completion_tokens": 50,
		"total_tokens":      150,
		"prompt_tokens_details": map[string]any{
			"cached_tokens": 20,
		},
		"cache_creation_input_tokens": 10,
	}

	result := provider.convertUsage(usage)

	assert.Equal(t, 100, result["input_tokens"])
	assert.Equal(t, 50, result["output_tokens"])
	assert.Equal(t, 20, result["cache_read_input_tokens"])
	assert.Equal(t, 10, result["cache_creation_input_tokens"])
}

func TestNvidiaProvider_ConvertToolCallID(t *testing.T) {
	provider := NewNvidiaProvider()

	tests := []struct {
		input    string
		expected string
	}{
		{"call_nvidia123", "toolu_nvidia123"},
		{"toolu_nvidia123", "toolu_nvidia123"},
		{"xyz789", "toolu_xyz789"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := provider.convertToolCallID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNvidiaProvider_MapNvidiaErrorType(t *testing.T) {
	provider := NewNvidiaProvider()

	tests := []struct {
		nvidiaType        string
		expectedAnthropic string
	}{
		{"invalid_request_error", "invalid_request_error"},
		{"authentication_error", "authentication_error"},
		{"permission_error", "permission_error"},
		{"not_found_error", "not_found_error"},
		{"rate_limit_error", "rate_limit_error"},
		{"api_error", "api_error"},
		{"overloaded_error", "overloaded_error"},
		{"insufficient_quota_error", "billing_error"},
		{"unknown_error", "api_error"},
	}

	for _, tt := range tests {
		t.Run(tt.nvidiaType, func(t *testing.T) {
			result := provider.mapNvidiaErrorType(tt.nvidiaType)
			assert.Equal(t, tt.expectedAnthropic, result)
		})
	}
}
