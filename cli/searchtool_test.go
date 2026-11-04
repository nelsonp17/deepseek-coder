package main

import (
	"strings"
	"testing"

	"deekseep-coder/internal/session"
)

// The search tool looks in the real session directory, so the test points
// LOCALAPPDATA at a temporary folder and writes a session there.
func withTempSessions(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_DATA_HOME", dir)
	return dir
}

func TestSearchToolFindsPastWork(t *testing.T) {
	dir := withTempSessions(t)
	s, err := session.Create(session.Dir(), session.Meta{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	s.AppendUser("arreglamos el endpoint /health con time.Since")
	s.AppendTurnEnd("completed")
	id := s.ID()
	s.Close()
	if !strings.Contains(session.Dir(), dir) {
		t.Fatalf("the session dir %q does not follow the override", session.Dir())
	}

	a := testApp()
	out, handled, err := a.executeSearchTool(toolCall(toolSearch, `{"query":"health"}`))
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if !strings.Contains(out, "health") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, id) {
		t.Errorf("output does not name the session %q:\n%s", id, out)
	}
}

func TestSearchToolWithoutMatches(t *testing.T) {
	withTempSessions(t)
	a := testApp()
	out, _, err := a.executeSearchTool(toolCall(toolSearch, `{"query":"nada-de-esto"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No matches") {
		t.Errorf("output = %q", out)
	}
}

func TestSearchToolValidation(t *testing.T) {
	withTempSessions(t)
	a := testApp()
	if _, _, err := a.executeSearchTool(toolCall(toolSearch, `{"query":"  "}`)); err == nil {
		t.Error("an empty query must be rejected")
	}
	if _, handled, _ := a.executeSearchTool(toolCall(toolRead, "{}")); handled {
		t.Error("a non-search call must not be handled")
	}
}

// Searching the past is reading, so it never asks for permission.
func TestSearchToolNeedsNoApproval(t *testing.T) {
	a := testApp()
	a.approvalAuto = false
	if a.callNeedsApproval(toolCall(toolSearch, `{"query":"x"}`)) {
		t.Error("session_search must not ask for approval")
	}
}

func TestFormatHits(t *testing.T) {
	if got := formatHits(nil); !strings.Contains(got, "No matches") {
		t.Errorf("empty = %q", got)
	}
}
