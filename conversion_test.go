package claudecodeproxy

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestConvertClaudeToOAIAndBack(t *testing.T) {
	claudeReq := ClaudeMessagesRequest{
		Model:     "claude-3-sonnet-20240229",
		MaxTokens: 256,
		Messages: []ClaudeMessage{
			{
				Role:    "user",
				Content: "Hello, Claude!",
			},
		},
	}

	oaiReq, err := ConvertClaudeToOAI(claudeReq)
	if err != nil {
		t.Fatalf("ConvertClaudeToOAI error: %v", err)
	}
	if oaiReq.Model != "gpt-4.1" {
		t.Errorf("Model mismatch: got %s, want %s", oaiReq.Model, "gpt-4.1")
	}
	if len(oaiReq.Messages) != 1 || len(oaiReq.Messages[0].Content) != 1 || oaiReq.Messages[0].Content[0].Text != "Hello, Claude!" {
		t.Errorf("Message content mismatch: got %+v", oaiReq.Messages)
	}

	claudeReq2, err := ConvertOAIToClaude(oaiReq)
	if err != nil {
		t.Fatalf("ConvertOAIToClaude error: %v", err)
	}
	if claudeReq2.Model != "gpt-4.1" {
		t.Errorf("Roundtrip model mismatch: got %s, want %s", claudeReq2.Model, "gpt-4.1")
	}
	if len(claudeReq2.Messages) != 1 {
		t.Errorf("Roundtrip message count mismatch: got %d, want 1", len(claudeReq2.Messages))
	}
	if blocks, ok := claudeReq2.Messages[0].Content.([]ClaudeContentBlockText); ok {
		if len(blocks) != 1 || blocks[0].Text != "Hello, Claude!" {
			t.Errorf("Roundtrip content block mismatch: got %+v", blocks)
		}
	} else {
		t.Errorf("Roundtrip content type mismatch: got %T", claudeReq2.Messages[0].Content)
	}
}

func TestConvertToolChoiceClaudeToOAI(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  any
	}{
		{
			name:  "auto",
			input: map[string]any{"type": "auto"},
			want:  "auto",
		},
		{
			name:  "any",
			input: map[string]any{"type": "any"},
			want:  "any",
		},
		{
			name:  "tool with name",
			input: map[string]any{"type": "tool", "name": "calculator"},
			want:  map[string]any{"type": "function", "function": map[string]any{"name": "calculator"}},
		},
		{
			name:  "nil",
			input: nil,
			want:  nil,
		},
		{
			name:  "unknown type",
			input: map[string]any{"type": "unknown"},
			want:  "auto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertToolChoiceClaudeToOAI(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertToolChoiceClaudeToOAI(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// Existing tests...

func TestNoBlankTextBlockInClaudeResponse(t *testing.T) {
	// Simulate a Claude stream with a blank text block and a tool_use block
	claudeStream := `
{"event":"message_start","data":{"type":"message_start","message":{"id":"msg_000000000000000000000000","type":"message","role":"assistant","model":"claude-3-7-sonnet-20250219","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0}}}
{"event":"content_block_start","data":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}
{"event":"content_block_stop","data":{"type":"content_block_stop","index":0}}
{"event":"content_block_start","data":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_aNta7TNMC8U07NRUaIe7GLNl","name":"Bash","input":{}}}}
{"event":"content_block_delta","data":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"echo hello world\",\"description\":\"Echoes 'hello world' to the terminal\"}"}}}
{"event":"content_block_stop","data":{"type":"content_block_stop","index":1}}
{"event":"message_delta","data":{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":0}}}
{"event":"message_stop","data":{"type":"message_stop"}}
`
	r := strings.NewReader(claudeStream)
	resp, err := ParseClaudeStreamToResponse(r)
	if err != nil {
		t.Fatalf("ParseClaudeStreamToResponse error: %v", err)
	}
	// Should not contain a blank text block
	for _, block := range resp.Content {
		if m, ok := block.(map[string]any); ok {
			if m["type"] == "text" && m["text"] == "" {
				t.Errorf("Found blank text block in Claude response: %+v", m)
			}
		}
	}
}

func TestConvertOAIStreamToClaudeStream_ToolUseStreamingToNonStream(t *testing.T) {
	// This test simulates a streaming OpenAI response with a tool call, then parses it to a non-stream Claude response.
	oaiStream := `
data: {"choices":[],"created":0,"id":"","prompt_filter_results":[{"content_filter_results":{"hate":{"filtered":false,"severity":"safe"},"self_harm":{"filtered":false,"severity":"safe"},"sexual":{"filtered":false,"severity":"safe"},"violence":{"filtered":false,"severity":"safe"}},"prompt_index":0}]}
data: {"choices":[{"index":0,"delta":{"content":null,"role":"assistant","tool_calls":[{"function":{"arguments":"","name":"Bash"},"id":"call_wt8tlK30LhNXN1NENYUGa2zA","index":0,"type":"function"}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"{\""},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"command"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"\":\""},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"echo"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":" hello"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":" world"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"\",\""},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"description"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"\":\""},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"Print"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"s"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":" '"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"hello"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":" world"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"'"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":" to"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":" the"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":" console"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"function":{"arguments":"\"}"},"index":0}]}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: {"choices":[{"finish_reason":"tool_calls","index":0,"delta":{"content":null}}],"created":1747436830,"id":"chatcmpl-BXyQwO8mFeDq7LEd95HIhyB1QOiSr","usage":{"completion_tokens":29,"completion_tokens_details":{"accepted_prediction_tokens":0,"rejected_prediction_tokens":0},"prompt_tokens":16562,"prompt_tokens_details":{"cached_tokens":16256},"total_tokens":16591},"model":"gpt-4.1-2025-04-14","system_fingerprint":"fp_3dfb47c1f3"}
data: [DONE]
`
	r := strings.NewReader(oaiStream)
	var w bytes.Buffer

	// Convert streaming OAI to Claude stream
	err := ConvertOAIStreamToClaudeStream(r, &w, "claude-3-sonnet-20240229")
	if err != nil {
		t.Fatalf("ConvertOAIStreamToClaudeStream error: %v", err)
	}
	// Now parse the buffered events to reconstruct a ClaudeMessagesResponse
	claudeResp, err := ParseClaudeStreamToResponse(&w)
	if err != nil {
		t.Fatalf("ParseClaudeStreamToResponse error: %v", err)
	}

	// Check that the response contains exactly one tool_use content block with expected fields
	toolUseBlocks := 0
	for _, block := range claudeResp.Content {
		if tub, ok := block.(*ClaudeContentBlockToolUse); ok {
			toolUseBlocks++
			// Check that the tool_use block has a name and id
			if tub.Name != "Bash" {
				t.Errorf("tool_use block name mismatch: got %v, want Bash", tub.Name)
			}
			if tub.ID != "call_wt8tlK30LhNXN1NENYUGa2zA" {
				t.Errorf("tool_use block id mismatch: got %v, want call_wt8tlK30LhNXN1NENYUGa2zA", tub.ID)
			}
			// Check that the input field is set and matches the expected arguments
			if tub.Input["command"] != "echo hello world" {
				t.Errorf("tool_use block input.command mismatch: got %v, want 'echo hello world'", tub.Input["command"])
			}
			if tub.Input["description"] != "Prints 'hello world' to the console" {
				t.Errorf("tool_use block input.description mismatch: got %v, want \"Prints 'hello world' to the console\"", tub.Input["description"])
			}
		}
	}
	if toolUseBlocks != 1 {
		content, _ := json.MarshalIndent(claudeResp.Content, " ", " ")
		t.Errorf("Expected exactly one tool_use content block in Claude response, got: %s", string(content))
	}

	// Check that the stop_reason is "tool_use"
	if claudeResp.StopReason == nil || *claudeResp.StopReason != "tool_use" {
		t.Errorf("Expected stop_reason 'tool_use', got: %v", claudeResp.StopReason)
	}
}

func TestConvertOAIStreamToClaudeStream_TextOnly(t *testing.T) {
	// Simulate a simple OpenAI streaming response with only text deltas
	oaiStream := `
data: {"id":"cmpl-abc","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"delta":{"content":"Hello, "}}]}
data: {"id":"cmpl-abc","object":"chat.completion.chunk","created":124,"model":"gpt-4o","choices":[{"delta":{"content":"world!"},"finish_reason":"stop"}]}
data: [DONE]
`
	r := strings.NewReader(oaiStream)
	var w bytes.Buffer

	err := ConvertOAIStreamToClaudeStream(r, &w, "claude-3-sonnet-20240229")
	if err != nil {
		t.Fatalf("ConvertOAIStreamToClaudeStream error: %v", err)
	}
	out := w.String()
	if !strings.Contains(out, `"event":"content_block_delta"`) && !strings.Contains(out, `"event": "content_block_delta"`) {
		t.Errorf("Expected content_block_delta event in output, got: %s", out)
	}
	if !strings.Contains(out, "Hello, ") || !strings.Contains(out, "world!") {
		t.Errorf("Expected streamed text in output, got: %s", out)
	}
	if !strings.Contains(out, "message_stop") {
		t.Errorf("Expected message_stop event in output, got: %s", out)
	}
}

func TestConvertOAIStreamToClaudeStream_FinishReason(t *testing.T) {
	// Simulate a stream with a finish_reason of "length"
	oaiStream := `
data: {"id":"cmpl-abc","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"delta":{"content":"Test"},"finish_reason":"length"}]}
data: [DONE]
`
	r := strings.NewReader(oaiStream)
	var w bytes.Buffer

	err := ConvertOAIStreamToClaudeStream(r, &w, "claude-3-sonnet-20240229")
	if err != nil {
		t.Fatalf("ConvertOAIStreamToClaudeStream error: %v", err)
	}
	out := w.String()
	if !strings.Contains(out, `"stop_reason":"max_tokens"`) {
		t.Errorf("Expected stop_reason to be max_tokens, got: %s", out)
	}
}

func TestConvertOAIStreamToClaudeStream_EmptyStream(t *testing.T) {
	// Simulate an empty stream (should still emit message_start etc.)
	oaiStream := `data: [DONE]
`
	r := strings.NewReader(oaiStream)
	var w bytes.Buffer

	err := ConvertOAIStreamToClaudeStream(r, &w, "claude-3-sonnet-20240229")
	if err != nil && err != io.EOF {
		t.Fatalf("ConvertOAIStreamToClaudeStream error: %v", err)
	}
	out := w.String()
	if !strings.Contains(out, "message_start") {
		t.Errorf("Expected message_start event in output, got: %s", out)
	}
	if !strings.Contains(out, "message_stop") {
		t.Errorf("Expected message_stop event in output, got: %s", out)
	}
}

func TestConvertOAIStreamToClaudeStream_ToolCall(t *testing.T) {
	// Simulate a stream with a tool call (tool_calls in delta)
	oaiStream := `
data: {"id":"cmpl-abc","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"delta":{"tool_calls":[{"id":"call_1","function":{"name":"calculator","arguments":"{\"expression\": \"2+2\"}"}}]}}]}
data: {"id":"cmpl-abc","object":"chat.completion.chunk","created":124,"model":"gpt-4o","choices":[{"delta":{},"finish_reason":"tool_calls"}]}
data: [DONE]
`
	r := strings.NewReader(oaiStream)
	var w bytes.Buffer

	err := ConvertOAIStreamToClaudeStream(r, &w, "claude-3-sonnet-20240229")
	if err != nil {
		t.Fatalf("ConvertOAIStreamToClaudeStream error: %v", err)
	}
	out := w.String()
	if !strings.Contains(out, `"type":"tool_use"`) && !strings.Contains(out, `"type": "tool_use"`) {
		t.Errorf("Expected tool_use content_block_start in output, got: %s", out)
	}
	if !strings.Contains(out, "input_json_delta") {
		t.Errorf("Expected input_json_delta in output, got: %s", out)
	}
	if !strings.Contains(out, "tool_calls") && !strings.Contains(out, "tool_use") {
		t.Errorf("Expected tool_calls/tool_use finish_reason in output, got: %s", out)
	}
}
