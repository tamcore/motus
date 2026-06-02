package chat

// Message is the wire representation of a chat message exchanged between the
// browser and the SSE endpoint.
type Message struct {
	Role    string `json:"role"` // "user" | "assistant" | "tool"
	Content string `json:"content"`

	// Tool call tracking (assistant messages with tool calls).
	ToolCallID string     `json:"toolCallId,omitempty"`
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
}

// ToolCall captures a single function invocation issued by the model.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatEvent is the SSE payload emitted by the streaming endpoint.
//
// Type values: "token", "tool_call", "tool_result", "done", "error".
type ChatEvent struct {
	Type    string `json:"type"`
	Delta   string `json:"delta,omitempty"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}
