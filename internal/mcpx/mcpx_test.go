package mcpx

import (
	"context"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestMCPServerHelper runs a real MCP server over stdio when spawned as a
// subprocess by the tests below.
func TestMCPServerHelper(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER") != "1" {
		return
	}
	s := server.NewMCPServer("test-server", "1.0.0")
	s.AddTool(mcp.NewTool("echo",
		mcp.WithDescription("Echoes the given text"),
		mcp.WithString("text", mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := req.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText("echo: " + text), nil
	})
	if err := server.ServeStdio(s); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestConnectAndCallTool(t *testing.T) {
	cfg := &Config{Servers: []ServerConfig{{
		Name:    "test",
		Command: os.Args[0],
		Args:    []string{"-test.run", "TestMCPServerHelper"},
		Env:     []string{"GO_WANT_MCP_HELPER=1"},
	}}}

	ctx := context.Background()
	mgr, err := Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer mgr.Close()

	servers := mgr.ConnectedServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 connected server, got %d", len(servers))
	}
	srv := servers[0]
	if srv.Name != "test" {
		t.Errorf("server name = %q", srv.Name)
	}
	if len(srv.Tools()) != 1 || srv.Tools()[0].Name != "echo" {
		t.Fatalf("tools = %+v, want [echo]", srv.Tools())
	}

	out, err := srv.CallTool(ctx, "echo", map[string]any{"text": "hola"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if out != "echo: hola" {
		t.Errorf("output = %q, want %q", out, "echo: hola")
	}

	if _, ok := mgr.FindTool("echo"); !ok {
		t.Error("FindTool(echo) not found")
	}
	if _, ok := mgr.FindTool("nope"); ok {
		t.Error("FindTool(nope) should not be found")
	}
}

func TestConnectSkipsBrokenServer(t *testing.T) {
	cfg := &Config{Servers: []ServerConfig{
		{Name: "broken", Command: "definitely-not-a-real-command-xyz"},
		{Name: "test", Command: os.Args[0], Args: []string{"-test.run", "TestMCPServerHelper"}, Env: []string{"GO_WANT_MCP_HELPER=1"}},
	}}

	ctx := context.Background()
	mgr, err := Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer mgr.Close()

	if len(mgr.ConnectedServers()) != 1 {
		t.Errorf("expected 1 connected server, got %d", len(mgr.ConnectedServers()))
	}
	if len(mgr.Servers()) != 2 {
		t.Errorf("expected 2 configured servers, got %d", len(mgr.Servers()))
	}
	// The broken server reports its error.
	for _, s := range mgr.Servers() {
		if s.Name == "broken" && s.ConnectErr() == nil {
			t.Error("broken server should report ConnectErr")
		}
	}
}

func TestToolResultText(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: "line1"},
			mcp.TextContent{Type: "text", Text: "line2"},
		},
	}
	if got := toolResultText(result); got != "line1\nline2" {
		t.Errorf("toolResultText = %q", got)
	}
}
