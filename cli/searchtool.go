package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"deekseep-coder/internal/deepseek"
	"deekseep-coder/internal/session"
)

// toolSearch is the tool that looks through past sessions.
const toolSearch = "session_search"

// searchTool describes session_search.
func (a *app) searchTool() deepseek.Tool {
	var params map[string]any
	_ = json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Text to look for in previous conversations. Case-insensitive."
			},
			"limit": {
				"type": "integer",
				"description": "Maximum matches to return (default 10, at most 20)."
			}
		},
		"required": ["query"]
	}`), &params)

	return deepseek.Tool{
		Type: "function",
		Function: deepseek.ToolFunction{
			Name:        toolSearch,
			Description: "Search previous conversations saved on this machine and return matching excerpts with the session id. Use it when the user refers to earlier work ('ayer arreglamos...', 'the migration we did'). Read-only.",
			Parameters:  params,
		},
	}
}

// executeSearchTool runs a past-session search.
func (a *app) executeSearchTool(tc deepseek.ToolCall) (string, bool, error) {
	if tc.Function.Name != toolSearch {
		return "", false, nil
	}
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := decodeArgs(tc.Function.Arguments, &args); err != nil {
		return "", true, err
	}
	if strings.TrimSpace(args.Query) == "" {
		return "", true, fmt.Errorf("the query cannot be empty")
	}
	hits, err := session.Search(session.Dir(), args.Query, args.Limit)
	if err != nil {
		return "", true, err
	}
	return formatHits(hits), true, nil
}

// formatHits renders search results.
func formatHits(hits []session.Hit) string {
	if len(hits) == 0 {
		return "No matches in previous conversations."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d match(es) in previous conversations:\n", len(hits))
	for _, hit := range hits {
		fmt.Fprintf(&b, "\n[%s · %s · %s]\n%s\n",
			hit.Time.Format("2006-01-02 15:04"), hit.SessionID, hit.Role, hit.Snippet)
	}
	b.WriteString("\nTo reopen one: deekseep --resume <session id>")
	return b.String()
}

// historySearch renders results for the `history search` command.
func (a *app) historySearch(query string) error {
	hits, err := session.Search(session.Dir(), query, 0)
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Println(a.msg.NoMatches)
		return nil
	}
	for _, hit := range hits {
		fmt.Printf("%s  %s  [%s]  %s\n",
			hit.Time.Format("2006-01-02 15:04"), hit.SessionID, hit.Role, hit.Snippet)
	}
	return nil
}
