package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("unexpected auth header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ModelsResponse{
			Object: "list",
			Data: []Model{
				{ID: "deepseek-chat", Name: "DeepSeek-V3", ContextWindow: 65536, MaxOutputTokens: 8192},
				{ID: "deepseek-reasoner", Name: "DeepSeek-R1", ContextWindow: 65536, MaxOutputTokens: 8192},
			},
		})
	}))
	defer server.Close()

	client := New(Options{APIKey: "test-key", BaseURL: server.URL})
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "deepseek-chat" || models[0].Name != "DeepSeek-V3" {
		t.Errorf("unexpected model: %+v", models[0])
	}
	if models[0].ContextWindow != 65536 || models[0].MaxOutputTokens != 8192 {
		t.Errorf("unexpected metadata: %+v", models[0])
	}
}

func TestStreamChatReasoning(t *testing.T) {
	chunks := []string{
		`{"id":"1","choices":[{"index":0,"delta":{"reasoning_content":"Voy a pensar"},"finish_reason":null}]}`,
		`{"id":"1","choices":[{"index":0,"delta":{"content":"Respuesta final"},"finish_reason":null}]}`,
		`{"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := New(Options{APIKey: "test-key", BaseURL: server.URL})
	var reasoning strings.Builder
	result, err := client.StreamChat(context.Background(), ChatRequest{
		Model:    "deepseek-reasoner",
		Messages: []Message{{Role: "user", Content: "hola"}},
	}, nil, func(d string) {
		reasoning.WriteString(d)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if result.Content != "Respuesta final" {
		t.Errorf("content = %q", result.Content)
	}
	if result.ReasoningContent != "Voy a pensar" {
		t.Errorf("reasoning = %q", result.ReasoningContent)
	}
	if reasoning.String() != "Voy a pensar" {
		t.Errorf("onReasoning got %q", reasoning.String())
	}
}

func TestListModelsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer server.Close()

	client := New(Options{APIKey: "bad", BaseURL: server.URL})
	if _, err := client.ListModels(context.Background()); err == nil {
		t.Fatal("expected error for unauthorized request")
	}
}

func TestStreamChatWatchdog(t *testing.T) {
	// The server sends one chunk and then stalls (never closes).
	oldTimeout := streamIdleTimeout
	streamIdleTimeout = 300 * time.Millisecond
	defer func() { streamIdleTimeout = oldTimeout }()

	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":"Hola"},"finish_reason":null}]}`+"\n\n")
		w.(http.Flusher).Flush()
		<-done // stall until the test finishes
	}))
	defer server.Close()
	defer close(done)

	client := New(Options{APIKey: "test-key", BaseURL: server.URL})
	_, err := client.StreamChat(context.Background(), ChatRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: "user", Content: "hola"}},
	}, nil)
	if err == nil {
		t.Fatal("expected idle timeout error")
	}
	if !strings.Contains(err.Error(), "idle timeout") {
		t.Errorf("error = %v, want idle timeout", err)
	}
}

func TestStreamChatToolCalls(t *testing.T) {
	// Streaming response with tool call fragments split across chunks,
	// followed by a final content chunk.
	chunks := []string{
		`{"id":"1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":""}}]},"finish_reason":null}]}`,
		`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"text\":"}}]},"finish_reason":null}]}`,
		`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"hola\"}"}}]},"finish_reason":null}]}`,
		`{"id":"1","choices":[{"index":0,"delta":{"content":"Listo"},"finish_reason":null}]}`,
		`{"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := New(Options{APIKey: "test-key", BaseURL: server.URL})
	var streamed strings.Builder
	result, err := client.StreamChat(context.Background(), ChatRequest{
		Model: "deepseek-chat",
		Messages: []Message{
			{Role: "user", Content: "usa la herramienta echo"},
		},
		Tools: []Tool{{Type: "function", Function: ToolFunction{Name: "echo", Description: "echo", Parameters: map[string]any{"type": "object"}}}},
	}, func(delta string) {
		streamed.WriteString(delta)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	if streamed.String() != "Listo" {
		t.Errorf("streamed content = %q, want Listo", streamed.String())
	}
	if result.Content != "Listo" {
		t.Errorf("result content = %q, want Listo", result.Content)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc.ID != "call_1" || tc.Function.Name != "echo" {
		t.Errorf("tool call = %+v", tc)
	}
	if tc.Function.Arguments != `{"text":"hola"}` {
		t.Errorf("arguments = %q, want %q", tc.Function.Arguments, `{"text":"hola"}`)
	}
}
