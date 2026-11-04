package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"deepseek-coder/internal/deepseek"
	"deepseek-coder/internal/fstools"
)

// Names of the built-in file tools. They are prefixed with _file where a bare
// name would be too generic, so they cannot collide with an MCP tool.
const (
	toolRead  = "read_file"
	toolWrite = "write_file"
	toolEdit  = "edit_file"
	toolGlob  = "glob"
	toolGrep  = "grep"
)

// fileSet returns the workspace tool set, creating it on first use.
func (a *app) fileSet() *fstools.Set {
	if a.fs == nil {
		set, err := fstools.New(".")
		if err != nil {
			return nil
		}
		a.fs = set
	}
	return a.fs
}

// toolNeedsApproval reports whether a call must be confirmed with the user.
// Reading file content is not a side effect; writing and running commands are.
func (a *app) toolNeedsApproval(name string) bool {
	switch name {
	case toolRead, toolGlob, toolGrep:
		// Reading is not a side effect.
		return false
	case toolTodo, toolQuestion, toolSearch:
		// Bookkeeping and asking: they touch the session log and the user, not
		// the workspace.
		return false
	}
	// exit_plan_mode is handled in callNeedsApproval: approving a plan is
	// always a human decision.
	return true
}

// fileTools describes the built-in file tools to the model.
func (a *app) fileTools() []deepseek.Tool {
	if a.fileSet() == nil {
		return nil
	}
	return []deepseek.Tool{
		{Type: "function", Function: deepseek.ToolFunction{
			Name: toolRead,
			Description: "Read a text file of the workspace as numbered lines. Use it instead of a shell command: " +
				"files must be read before they can be edited, and this reports line numbers, pagination and " +
				"binary/large-file refusals.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Path relative to the workspace root."},
					"offset": map[string]any{"type": "integer", "description": "First line to return, 1-based. Omit to start at the beginning."},
					"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to return (default and maximum 2000)."},
				},
				"required": []string{"path"},
			},
		}},
		{Type: "function", Function: deepseek.ToolFunction{
			Name: toolWrite,
			Description: "Create a file, or replace one completely. An existing file must have been read first and must " +
				"be unchanged, which prevents overwriting content the model never saw. Prefer edit_file for small changes.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Path relative to the workspace root."},
					"content": map[string]any{"type": "string", "description": "Full new content of the file."},
				},
				"required": []string{"path", "content"},
			},
		}},
		{Type: "function", Function: deepseek.ToolFunction{
			Name: toolEdit,
			Description: "Replace a literal snippet in an existing file. The snippet must appear exactly once unless " +
				"replace_all is set, and the file must have been read first. Use it for targeted changes: it keeps the " +
				"rest of the file untouched.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": "Path relative to the workspace root."},
					"old_string":  map[string]any{"type": "string", "description": "Exact text to replace, including its indentation."},
					"new_string":  map[string]any{"type": "string", "description": "Replacement text."},
					"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence instead of requiring a unique match."},
				},
				"required": []string{"path", "old_string", "new_string"},
			},
		}},
		{Type: "function", Function: deepseek.ToolFunction{
			Name: toolGlob,
			Description: "Find files whose path matches a glob pattern. Supports * and ** (any number of directories). " +
				"Results are newest first and capped.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": `Glob pattern such as "**/*.go" or "cli/*.go".`},
					"limit":   map[string]any{"type": "integer", "description": "Maximum number of files (default and maximum 100)."},
				},
				"required": []string{"pattern"},
			},
		}},
		{Type: "function", Function: deepseek.ToolFunction{
			Name: toolGrep,
			Description: "Search file contents with a regular expression and return file:line: text. " +
				"Skips binary and very large files and reports the match cap when it is reached.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "Go regular expression."},
					"path":    map[string]any{"type": "string", "description": "File or directory to search, relative to the workspace root. Omit for the whole workspace."},
					"limit":   map[string]any{"type": "integer", "description": "Maximum number of matches (default and maximum 250)."},
				},
				"required": []string{"pattern"},
			},
		}},
	}
}

// executeFileTool runs a built-in file tool. It reports handled=false when the
// call is not one of them.
func (a *app) executeFileTool(tc deepseek.ToolCall) (output string, handled bool, err error) {
	set := a.fileSet()
	if set == nil {
		return "", false, nil
	}
	switch tc.Function.Name {
	case toolRead:
		var args struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		if err := decodeArgs(tc.Function.Arguments, &args); err != nil {
			return "", true, err
		}
		out, err := set.Read(args.Path, args.Offset, args.Limit)
		return out, true, err

	case toolWrite:
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := decodeArgs(tc.Function.Arguments, &args); err != nil {
			return "", true, err
		}
		out, err := set.Write(args.Path, args.Content)
		return out, true, err

	case toolEdit:
		var args struct {
			Path       string `json:"path"`
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		}
		if err := decodeArgs(tc.Function.Arguments, &args); err != nil {
			return "", true, err
		}
		out, err := set.Edit(args.Path, args.OldString, args.NewString, args.ReplaceAll)
		return out, true, err

	case toolGlob:
		var args struct {
			Pattern string `json:"pattern"`
			Limit   int    `json:"limit"`
		}
		if err := decodeArgs(tc.Function.Arguments, &args); err != nil {
			return "", true, err
		}
		out, err := set.Glob(args.Pattern, args.Limit)
		return out, true, err

	case toolGrep:
		var args struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
			Limit   int    `json:"limit"`
		}
		if err := decodeArgs(tc.Function.Arguments, &args); err != nil {
			return "", true, err
		}
		out, err := set.Grep(args.Pattern, args.Path, args.Limit)
		return out, true, err
	}
	return "", false, nil
}

// decodeArgs unmarshals tool arguments, tolerating an empty payload.
func decodeArgs(raw string, out any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}

// fileToolPreview renders a readable preview of a write/edit call for the
// permission prompt, so the user approves a change and not a JSON blob.
func fileToolPreview(name, rawArgs string) (string, bool) {
	switch name {
	case toolWrite:
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if decodeArgs(rawArgs, &args) != nil {
			return "", false
		}
		return fmt.Sprintf("%s  (%s, %d lines)\n%s",
			args.Path, humanSize(len(args.Content)), countTextLines(args.Content),
			previewLines(args.Content, "+ ")), true

	case toolEdit:
		var args struct {
			Path       string `json:"path"`
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		}
		if decodeArgs(rawArgs, &args) != nil {
			return "", false
		}
		scope := ""
		if args.ReplaceAll {
			scope = "  (todas las apariciones)"
		}
		return fmt.Sprintf("%s%s\n%s\n%s",
			args.Path, scope,
			previewLines(args.OldString, "- "),
			previewLines(args.NewString, "+ ")), true
	}
	return "", false
}

// previewLines prefixes at most maxPreviewLines lines of text.
func previewLines(text string, prefix string) string {
	const maxPreviewLines = 12
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return prefix + "(vacío)"
	}
	var b strings.Builder
	for i, line := range lines {
		if i >= maxPreviewLines {
			fmt.Fprintf(&b, "%s… (%d líneas más)\n", prefix, len(lines)-maxPreviewLines)
			break
		}
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

// humanSize formats a byte count for a preview.
func humanSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// countTextLines counts lines in a string, ignoring a trailing newline.
func countTextLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
