package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// EventSink receives streaming events from the chat service.
type EventSink interface {
	Send(event ChatEvent) error
	Flush() error
}

// Service orchestrates streaming chat completions with MCP tool dispatch.
type Service struct {
	client      *openai.Client
	model       string
	maxTokens   int
	temperature float64
	sysPrompt   string
	maxLoops    int
	timeout     time.Duration
	mcpServer   *mcpserver.MCPServer
	tools       []openai.ChatCompletionToolParam
}

// Config holds the Service constructor arguments.
type Config struct {
	BaseURL      string
	APIKey       string
	Model        string
	MaxTokens    int
	Temperature  float64
	SystemPrompt string
	MaxLoops     int
	Timeout      time.Duration
	MCPServer    *mcpserver.MCPServer
}

// NewService creates a chat Service from the given Config.
func NewService(cfg Config) *Service {
	client := openai.NewClient(
		option.WithBaseURL(cfg.BaseURL),
		option.WithAPIKey(cfg.APIKey),
	)
	svc := &Service{
		client:      &client,
		model:       cfg.Model,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
		sysPrompt:   cfg.SystemPrompt,
		maxLoops:    cfg.MaxLoops,
		timeout:     cfg.Timeout,
		mcpServer:   cfg.MCPServer,
	}
	svc.tools = svc.buildTools()
	return svc
}

// buildTools converts all registered MCP tools into OpenAI tool params.
func (s *Service) buildTools() []openai.ChatCompletionToolParam {
	registered := s.mcpServer.ListTools()
	tools := make([]openai.ChatCompletionToolParam, 0, len(registered))
	for _, st := range registered {
		schema := shared.FunctionParameters{}
		if b, err := json.Marshal(st.Tool.InputSchema); err == nil {
			_ = json.Unmarshal(b, &schema)
		}
		tools = append(tools, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        st.Tool.Name,
				Description: param.NewOpt(st.Tool.Description),
				Parameters:  schema,
			},
		})
	}
	return tools
}

// HistoryHandle provides a conversation's message history and persists new
// turns. Implementations may be backed by Redis or held in memory only.
type HistoryHandle interface {
	// Messages returns the current conversation messages (excluding system prompt).
	Messages() []Message
	// Append persists one or more new messages. Errors are non-fatal — the
	// implementation must update its in-memory view regardless.
	Append(ctx context.Context, msgs ...Message) error
}

// Stream runs the chat loop, sending SSE events to sink until the model
// produces a final response or an error occurs.
func (s *Service) Stream(ctx context.Context, hist HistoryHandle, sink EventSink) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	history := s.buildHistory(hist.Messages())

	for loop := 0; loop < s.maxLoops; loop++ {
		pendingCalls, text, err := s.streamOnce(ctx, history, sink)
		if err != nil {
			_ = sink.Send(ChatEvent{Type: "error", Message: err.Error()})
			_ = sink.Flush()
			break
		}
		if len(pendingCalls) == 0 {
			// Final text response — persist it.
			if text != "" {
				_ = hist.Append(ctx, Message{Role: "assistant", Content: text})
			}
			break
		}

		// Persist the assistant turn (tool_calls only; text is display-only here).
		_ = hist.Append(ctx, Message{Role: "assistant", ToolCalls: pendingCalls})
		history = append(history, assistantMessageWithCalls(pendingCalls))

		for _, tc := range pendingCalls {
			_ = sink.Send(ChatEvent{Type: "tool_call", ID: tc.ID, Name: tc.Name})
			_ = sink.Flush()

			result, toolErr := s.dispatchTool(ctx, tc.Name, tc.Arguments)
			if toolErr != nil {
				errJSON := fmt.Sprintf(`{"error":%q}`, toolErr.Error())
				_ = sink.Send(ChatEvent{Type: "tool_result", ID: tc.ID, Name: tc.Name, Error: toolErr.Error()})
				_ = sink.Flush()
				_ = hist.Append(ctx, Message{Role: "tool", Content: errJSON, ToolCallID: tc.ID})
				history = append(history, openai.ToolMessage(errJSON, tc.ID))
				continue
			}

			_ = sink.Send(ChatEvent{Type: "tool_result", ID: tc.ID, Name: tc.Name, Result: json.RawMessage(result)})
			_ = sink.Flush()
			_ = hist.Append(ctx, Message{Role: "tool", Content: result, ToolCallID: tc.ID})
			history = append(history, openai.ToolMessage(result, tc.ID))
		}
	}

	_ = sink.Send(ChatEvent{Type: "done"})
	return sink.Flush()
}

// streamOnce performs a single streaming request, returning accumulated tool
// calls and the assistant's text content. When finish reason is not
// "tool_calls", pendingCalls is nil and text holds the full response.
func (s *Service) streamOnce(ctx context.Context, history []openai.ChatCompletionMessageParamUnion, sink EventSink) ([]ToolCall, string, error) {
	params := openai.ChatCompletionNewParams{
		Model:       s.model,
		Messages:    history,
		Tools:       s.tools,
		MaxTokens:   param.NewOpt(int64(s.maxTokens)),
		Temperature: param.NewOpt(s.temperature),
	}

	stream := s.client.Chat.Completions.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	type callBuf struct {
		id   string
		name string
		args strings.Builder
	}
	callBufs := map[int64]*callBuf{}
	finishReason := ""
	var textBuf strings.Builder

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		finishReason = choice.FinishReason

		if choice.Delta.Content != "" {
			textBuf.WriteString(choice.Delta.Content)
			_ = sink.Send(ChatEvent{Type: "token", Delta: choice.Delta.Content})
			_ = sink.Flush()
		}

		for _, tc := range choice.Delta.ToolCalls {
			buf, ok := callBufs[tc.Index]
			if !ok {
				buf = &callBuf{}
				callBufs[tc.Index] = buf
			}
			if tc.ID != "" {
				buf.id = tc.ID
			}
			if tc.Function.Name != "" {
				buf.name = tc.Function.Name
			}
			buf.args.WriteString(tc.Function.Arguments)
		}
	}

	if err := stream.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, "", fmt.Errorf("request timed out")
		}
		return nil, "", fmt.Errorf("stream error: %w", err)
	}

	if finishReason != "tool_calls" {
		return nil, textBuf.String(), nil
	}

	calls := make([]ToolCall, 0, len(callBufs))
	for i := int64(0); i < int64(len(callBufs)); i++ {
		buf, ok := callBufs[i]
		if !ok {
			continue
		}
		calls = append(calls, ToolCall{ID: buf.id, Name: buf.name, Arguments: buf.args.String()})
	}
	return calls, textBuf.String(), nil
}

// dispatchTool invokes an MCP tool by name and returns the JSON result string.
func (s *Service) dispatchTool(ctx context.Context, name, arguments string) (string, error) {
	st := s.mcpServer.GetTool(name)
	if st == nil {
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	var args map[string]any
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("invalid tool arguments: %w", err)
		}
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := st.Handler(ctx, req)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "{}", nil
	}

	if len(result.Content) == 1 {
		if tc, ok := result.Content[0].(mcp.TextContent); ok {
			return tc.Text, nil
		}
	}

	b, err := json.Marshal(result.Content)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// buildHistory converts input messages to OpenAI format, prepending the system prompt.
func (s *Service) buildHistory(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	sysMsg := s.sysPrompt
	if sysMsg == "" {
		sysMsg = "You are a helpful assistant for the motus GPS tracking platform. " +
			"The authenticated user can manage their own GPS devices, positions, geofences, calendars, and notification rules.\n\n" +
			"Available tool categories:\n" +
			"- Devices: list_devices\n" +
			"- Positions: get_latest_position\n" +
			"- Trips/events: get_distance_traveled, list_events\n" +
			"- Geofences: list_geofences, create_geofence, update_geofence, delete_geofence\n" +
			"- Calendars (time windows for geofences): list_calendars, create_calendar\n" +
			"- Notifications: list_notification_rules, create_notification_rule, update_notification_rule, delete_notification_rule\n" +
			"- Geocoding: geocode_address\n\n" +
			"Multi-step planning: when the user asks for a time-bound geofence (e.g. 'active on Fridays'), " +
			"first call create_calendar with the desired schedule, then call create_geofence with the returned calendar_id.\n\n" +
			"Relative dates: always call get_server_time first when the user says 'today', 'this Friday', 'last week', etc.\n\n" +
			"Today's date: " + time.Now().UTC().Format("2006-01-02") + "."
	}

	history := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs)+1)
	history = append(history, openai.SystemMessage(sysMsg))

	for _, m := range msgs {
		switch m.Role {
		case "user":
			history = append(history, openai.UserMessage(m.Content))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				history = append(history, assistantMessageWithCalls(m.ToolCalls))
			} else {
				history = append(history, openai.AssistantMessage(m.Content))
			}
		case "tool":
			history = append(history, openai.ToolMessage(m.Content, m.ToolCallID))
		}
	}
	return history
}

// assistantMessageWithCalls builds an assistant message that carries tool_calls.
func assistantMessageWithCalls(calls []ToolCall) openai.ChatCompletionMessageParamUnion {
	toolCallParams := make([]openai.ChatCompletionMessageToolCallParam, len(calls))
	for i, tc := range calls {
		toolCallParams[i] = openai.ChatCompletionMessageToolCallParam{
			ID: tc.ID,
			Function: openai.ChatCompletionMessageToolCallFunctionParam{
				Name:      tc.Name,
				Arguments: tc.Arguments,
			},
		}
	}
	return openai.ChatCompletionMessageParamUnion{
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			ToolCalls: toolCallParams,
		},
	}
}
