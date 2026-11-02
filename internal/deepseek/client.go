package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultBaseURL is the DeepSeek API endpoint.
	DefaultBaseURL = "https://api.deepseek.com"
	// DefaultModel is used when no model is configured.
	DefaultModel = "deepseek-chat"
)

// streamIdleTimeout aborts the stream if no data arrives within this window,
// so a stalled connection does not hang forever. It is a variable so tests
// can shorten it.
var streamIdleTimeout = 60 * time.Second

// Client talks to the DeepSeek API.
type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// Options configures a Client.
type Options struct {
	APIKey  string
	BaseURL string // defaults to DefaultBaseURL
	Model   string // defaults to DefaultModel
}

// New creates a Client.
func New(opts Options) *Client {
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	model := opts.Model
	if model == "" {
		model = DefaultModel
	}
	Debugf("client: baseURL=%s model=%s apiKeySet=%v", baseURL, model, opts.APIKey != "")
	return &Client{
		apiKey:  opts.APIKey,
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{Timeout: 5 * time.Minute},
	}
}

// Model returns the configured model.
func (c *Client) Model() string { return c.model }

// ListModels returns the models available on the API.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	start := time.Now()
	Debugf("GET %s/models", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		Debugf("GET /models error after %s: %v", time.Since(start), err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		Debugf("GET /models -> %s after %s", resp.Status, time.Since(start))
		return nil, apiError(resp)
	}
	var out ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	Debugf("GET /models -> %d models after %s", len(out.Data), time.Since(start))
	return out.Data, nil
}

// Chat sends a chat completion request and returns the full response.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	var out ChatResponse
	if err := c.doJSON(ctx, http.MethodPost, "/chat/completions", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StreamChat sends a streaming chat completion request and calls onChunk for
// each content delta. The optional onReasoning callback receives the model's
// reasoning content (used by reasoning models before they answer). Tool call
// fragments are accumulated across chunks and returned in the result.
func (c *Client) StreamChat(ctx context.Context, req ChatRequest, onChunk func(string), onReasoning ...func(string)) (*StreamResult, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	req.Stream = true

	start := time.Now()
	Debugf("POST %s/chat/completions model=%s messages=%d tools=%d", c.baseURL, req.Model, len(req.Messages), len(req.Tools))

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// The request uses a cancellable context so the watchdog can abort a
	// stalled stream.
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		Debugf("POST /chat/completions error after %s: %v", time.Since(start), err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		Debugf("POST /chat/completions -> %s after %s", resp.Status, time.Since(start))
		return nil, apiError(resp)
	}
	Debugf("POST /chat/completions -> %s, streaming...", resp.Status)

	// Watchdog: abort if the server stops sending data (stalled connection).
	var lastData atomic.Int64
	lastData.Store(time.Now().UnixNano())
	watchdogFired := make(chan struct{})
	fireWatchdog := sync.OnceFunc(func() { close(watchdogFired) })
	go func() {
		ticker := time.NewTicker(streamIdleTimeout / 4)
		defer ticker.Stop()
		for {
			select {
			case <-watchdogFired:
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, lastData.Load())) > streamIdleTimeout {
					Debugf("stream watchdog: no data for %s, aborting", streamIdleTimeout)
					fireWatchdog()
					cancel()
					return
				}
			}
		}
	}()
	defer fireWatchdog()

	var full strings.Builder
	var reasoning strings.Builder
	var toolCalls []ToolCall
	var reasonCb func(string)
	if len(onReasoning) > 0 && onReasoning[0] != nil {
		reasonCb = onReasoning[0]
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		// SSE lines look like "data: {...}" and end with "data: [DONE]".
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		lastData.Store(time.Now().UnixNano())
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk StreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue // skip malformed chunks
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				full.WriteString(ch.Delta.Content)
				if onChunk != nil {
					onChunk(ch.Delta.Content)
				}
			}
			if ch.Delta.ReasoningContent != "" {
				reasoning.WriteString(ch.Delta.ReasoningContent)
				if reasonCb != nil {
					reasonCb(ch.Delta.ReasoningContent)
				}
			}
			for _, tcd := range ch.Delta.ToolCalls {
				for len(toolCalls) <= tcd.Index {
					toolCalls = append(toolCalls, ToolCall{})
				}
				if tcd.ID != "" {
					toolCalls[tcd.Index].ID = tcd.ID
				}
				if tcd.Type != "" {
					toolCalls[tcd.Index].Type = tcd.Type
				}
				if tcd.Function.Name != "" {
					toolCalls[tcd.Index].Function.Name = tcd.Function.Name
				}
				toolCalls[tcd.Index].Function.Arguments += tcd.Function.Arguments
			}
		}
	}
	if err := scanner.Err(); err != nil {
		select {
		case <-watchdogFired:
			Debugf("stream aborted by watchdog after %s", time.Since(start))
			return nil, fmt.Errorf("stream idle timeout: no data for %s", streamIdleTimeout)
		default:
		}
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("stream aborted: %w", err)
		}
		Debugf("stream error after %s: %v", time.Since(start), err)
		return nil, err
	}
	Debugf("stream done after %s: content=%d bytes reasoning=%d bytes toolCalls=%d", time.Since(start), full.Len(), reasoning.Len(), len(toolCalls))
	return &StreamResult{Content: full.String(), ReasoningContent: reasoning.String(), ToolCalls: toolCalls}, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func apiError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("deepseek API error: %s: %s", resp.Status, strings.TrimSpace(string(data)))
}
