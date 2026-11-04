package deepseek

// Message is a single chat message.
type Message struct {
	Role       string     `json:"role"` // "system", "user", "assistant" or "tool"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolFunction describes a callable function.
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON schema
}

// Tool is a function definition sent to the API.
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolCall is a function call requested by the model.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON string
	} `json:"function"`
}

// ChatRequest is the request body for POST /chat/completions.
type ChatRequest struct {
	Model           string         `json:"model"`
	Messages        []Message      `json:"messages"`
	Stream          bool           `json:"stream,omitempty"`
	StreamOptions   *StreamOptions `json:"stream_options,omitempty"`
	Tools           []Tool         `json:"tools,omitempty"`
	Thinking        map[string]any `json:"thinking,omitempty"` // {"type": "enabled"|"disabled"}
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	Temperature     *float64       `json:"temperature,omitempty"`
	MaxTokens       *int           `json:"max_tokens,omitempty"`
}

// StreamOptions asks the API to report token usage on a streaming response.
// Without it the final chunk carries no usage at all.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// Usage reports token usage for a completion.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// Cache counters are reported by DeepSeek; they let the CLI show how much
	// of the prompt was served from the context cache.
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens,omitempty"`
}

// ChatResponse is the non-streaming response of /chat/completions.
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int     `json:"index"`
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

// Model describes a model offering from GET /models.
type Model struct {
	ID               string   `json:"id"`
	Object           string   `json:"object"`
	OwnedBy          string   `json:"owned_by"`
	Name             string   `json:"name"`
	ContextWindow    int      `json:"context_window"`
	MaxOutputTokens  int      `json:"max_output_tokens"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

// ModelsResponse is the response of GET /models.
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// ToolCallDelta is one streaming fragment of a tool call.
type ToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Delta is the streaming delta of a choice.
type Delta struct {
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content"`
	ToolCalls        []ToolCallDelta `json:"tool_calls"`
}

// StreamChunk is one SSE chunk of a streaming response.
type StreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Usage   *Usage `json:"usage,omitempty"`
	Choices []struct {
		Index        int     `json:"index"`
		Delta        Delta   `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// StreamResult is the outcome of one streaming chat turn.
type StreamResult struct {
	Content          string
	ReasoningContent string
	ToolCalls        []ToolCall
	// Usage is the token accounting reported by the API, when available.
	Usage *Usage
}

// BalanceInfo is the balance of one currency in the account.
type BalanceInfo struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

// BalanceResponse is the response of GET /user/balance.
type BalanceResponse struct {
	IsAvailable  bool          `json:"is_available"`
	BalanceInfos []BalanceInfo `json:"balance_infos"`
}
