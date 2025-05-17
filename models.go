package claudecodeproxy

// -------------------- Claude (Anthropic) API Structs --------------------

// ClaudeContentBlockText represents a text content block for Claude API.
type ClaudeContentBlockText struct {
	Type string `json:"type"` // always "text"
	Text string `json:"text"`
}

// ClaudeContentBlockImage represents an image content block for Claude API.
type ClaudeContentBlockImage struct {
	Type   string         `json:"type"` // always "image"
	Source map[string]any `json:"source"`
}

// ClaudeContentBlockToolUse represents a tool use content block for Claude API.
type ClaudeContentBlockToolUse struct {
	Type  string         `json:"type"` // always "tool_use"`
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// ClaudeContentBlockToolResult represents a tool result content block for Claude API.
type ClaudeContentBlockToolResult struct {
	Type      string `json:"type"` // always "tool_result"
	ToolUseID string `json:"tool_use_id"`
	Content   any    `json:"content"` // can be string, list, dict, etc.
}

// ClaudeSystemContent represents a system content block for Claude API.
type ClaudeSystemContent struct {
	Type string `json:"type"` // always "text"
	Text string `json:"text"`
}

// ClaudeMessage represents a chat message for Claude API.
type ClaudeMessage struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content any    `json:"content"` // string or list of content blocks
}

// ClaudeTool represents a tool definition for Claude API.
type ClaudeTool struct {
	Name        string         `json:"name"`
	Description *string        `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

// ClaudeThinkingConfig represents the thinking configuration for Claude API.
type ClaudeThinkingConfig struct {
	Enabled bool `json:"enabled"`
}

// ClaudeMessagesRequest represents the request body for /v1/messages (Claude API).
type ClaudeMessagesRequest struct {
	Model         string                `json:"model"`
	MaxTokens     int                   `json:"max_tokens"`
	Messages      []ClaudeMessage       `json:"messages"`
	System        any                   `json:"system,omitempty"` // string or []ClaudeSystemContent
	StopSequences *[]string             `json:"stop_sequences,omitempty"`
	Stream        *bool                 `json:"stream,omitempty"`
	Temperature   *float64              `json:"temperature,omitempty"`
	TopP          *float64              `json:"top_p,omitempty"`
	TopK          *int                  `json:"top_k,omitempty"`
	Metadata      *map[string]any       `json:"metadata,omitempty"`
	Tools         *[]ClaudeTool         `json:"tools,omitempty"`
	ToolChoice    *map[string]any       `json:"tool_choice,omitempty"`
	Thinking      *ClaudeThinkingConfig `json:"thinking,omitempty"`
	OriginalModel *string               `json:"original_model,omitempty"`
}

// ClaudeTokenCountRequest represents the request body for /v1/messages/count_tokens (Claude API).
type ClaudeTokenCountRequest struct {
	Model         string                `json:"model"`
	Messages      []ClaudeMessage       `json:"messages"`
	System        any                   `json:"system,omitempty"` // string or []ClaudeSystemContent
	Tools         *[]ClaudeTool         `json:"tools,omitempty"`
	Thinking      *ClaudeThinkingConfig `json:"thinking,omitempty"`
	ToolChoice    *map[string]any       `json:"tool_choice,omitempty"`
	OriginalModel *string               `json:"original_model,omitempty"`
}

// ClaudeTokenCountResponse represents the response for token counting (Claude API).
type ClaudeTokenCountResponse struct {
	InputTokens int `json:"input_tokens"`
}

// ClaudeUsage represents token usage statistics for Claude API.
type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ClaudeMessagesResponse represents the response body for /v1/messages (Claude API).
type ClaudeMessagesResponse struct {
	ID           string      `json:"id"`
	Model        string      `json:"model"`
	Role         string      `json:"role"`    // always "assistant"
	Content      []any       `json:"content"` // []ClaudeContentBlockText, []ClaudeContentBlockToolUse, etc.
	Type         string      `json:"type"`    // always "message"
	StopReason   *string     `json:"stop_reason,omitempty"`
	StopSequence *string     `json:"stop_sequence,omitempty"`
	Usage        ClaudeUsage `json:"usage"`
}

// -------------------- OpenAI/LiteLLM API Structs --------------------

// OAIMessage represents a chat message for OpenAI/LiteLLM API.
type OAIMessage struct {
	Role    string              `json:"role"` // "user", "assistant", "system", etc.
	Content []OAIMessageContent `json:"content"`
	// Optionally, you can add ToolCalls, Name, etc. if needed
}

type OAIMessageContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// OAIFunctionTool represents a function tool for OpenAI/LiteLLM API.
type OAIFunctionTool struct {
	Type     string         `json:"type"` // always "function"
	Function map[string]any `json:"function"`
}

// OAIRequest represents the request body for OpenAI/LiteLLM API.
type OAIRequest struct {
	Model       string             `json:"model"`
	Messages    []OAIMessage       `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	TopK        *int               `json:"top_k,omitempty"`
	Stop        *[]string          `json:"stop,omitempty"`
	Tools       *[]OAIFunctionTool `json:"tools,omitempty"`
	ToolChoice  any                `json:"tool_choice,omitempty"`
	Stream      bool               `json:"stream"`
	APIKey      *string            `json:"api_key,omitempty"`
}

// OAIUsage represents token usage statistics for OpenAI/LiteLLM API.
type OAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OAIResponse represents the response body for OpenAI/LiteLLM API.
type OAIResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []any    `json:"choices"`
	Usage   OAIUsage `json:"usage"`
}
