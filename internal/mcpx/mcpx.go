package mcpx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// ServerConfig describes one MCP server to launch over stdio.
type ServerConfig struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

// Config is the mcp.json file format.
type Config struct {
	Servers []ServerConfig `json:"servers"`
}

// LoadConfig reads the MCP configuration from the user config directory.
// A missing file means no MCP servers configured (not an error).
func LoadConfig() (*Config, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "deepseek-coder", "mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid mcp.json: %w", err)
	}
	return &cfg, nil
}

// Server is a connected MCP server. When connectErr is non-nil the server
// could not be started.
type Server struct {
	Name       string
	client     *client.Client
	tools      []mcp.Tool
	connectErr error
}

// ConnectErr returns the error that prevented the server from starting, or
// nil if it connected successfully.
func (s *Server) ConnectErr() error { return s.connectErr }

// Tools returns the tools exposed by this server.
func (s *Server) Tools() []mcp.Tool { return s.tools }

// CallTool invokes a tool and returns its text output.
func (s *Server) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	result, err := s.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: name, Arguments: args},
	})
	if err != nil {
		return "", err
	}
	out := toolResultText(result)
	if result.IsError {
		return "", fmt.Errorf("tool %s failed: %s", name, out)
	}
	return out, nil
}

// Manager holds all connected MCP servers.
type Manager struct {
	servers []*Server
}

// Connect launches and initializes every configured server. Servers that
// fail to start are skipped (with their error reported) so one broken server
// does not block the rest.
func Connect(ctx context.Context, cfg *Config) (*Manager, error) {
	m := &Manager{}
	for _, sc := range cfg.Servers {
		if strings.TrimSpace(sc.Name) == "" || strings.TrimSpace(sc.Command) == "" {
			continue
		}
		srv, err := connectServer(ctx, sc)
		if err != nil {
			m.servers = append(m.servers, &Server{Name: sc.Name, connectErr: err})
			continue
		}
		m.servers = append(m.servers, srv)
	}
	return m, nil
}

// Close shuts down all servers.
func (m *Manager) Close() {
	for _, s := range m.servers {
		if s.client != nil {
			_ = s.client.Close()
		}
	}
}

// Servers returns all configured servers (including failed ones).
func (m *Manager) Servers() []*Server { return m.servers }

// ConnectedServers returns only the servers that started successfully.
func (m *Manager) ConnectedServers() []*Server {
	var out []*Server
	for _, s := range m.servers {
		if s.client != nil {
			out = append(out, s)
		}
	}
	return out
}

// FindTool locates the connected server exposing a tool with the given name.
func (m *Manager) FindTool(name string) (*Server, bool) {
	for _, s := range m.servers {
		if s.client == nil {
			continue
		}
		for _, t := range s.tools {
			if t.Name == name {
				return s, true
			}
		}
	}
	return nil, false
}

func connectServer(parent context.Context, sc ServerConfig) (*Server, error) {
	// The transport's context must live for the whole session: the stdio
	// transport binds the subprocess to it (exec.CommandContext). Only the
	// handshake calls get a timeout.
	stdio := transport.NewStdio(sc.Command, sc.Env, sc.Args...)
	c := client.NewClient(stdio)
	if err := c.Start(parent); err != nil {
		return nil, fmt.Errorf("start %s: %w", sc.Name, err)
	}

	opCtx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if _, err := c.Initialize(opCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ClientInfo: mcp.Implementation{Name: "deepseek-coder", Version: "0.1.0"},
		},
	}); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("initialize %s: %w", sc.Name, err)
	}
	tools, err := c.ListTools(opCtx, mcp.ListToolsRequest{})
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("list tools %s: %w", sc.Name, err)
	}
	return &Server{Name: sc.Name, client: c, tools: tools.Tools}, nil
}

// toolResultText extracts the text content of a tool result.
func toolResultText(result *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range result.Content {
		switch v := c.(type) {
		case mcp.TextContent:
			b.WriteString(v.Text)
		case mcp.EmbeddedResource:
			if tr, ok := v.Resource.(mcp.TextResourceContents); ok {
				b.WriteString(tr.Text)
			}
		default:
			if data, err := json.Marshal(v); err == nil {
				b.Write(data)
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
