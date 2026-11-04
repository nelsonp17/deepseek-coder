package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deepseek-coder/internal/deepseek"
)

func TestGroupNumber(t *testing.T) {
	cases := map[int]string{
		0: "0", 999: "999", 1000: "1 000", 1234567: "1 234 567", -1234: "-1 234",
	}
	for n, want := range cases {
		if got := groupNumber(n); got != want {
			t.Errorf("groupNumber(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestCompactTokens(t *testing.T) {
	cases := map[int]string{999: "999", 12345: "12.3k", 1500000: "1.5M"}
	for n, want := range cases {
		if got := compactTokens(n); got != want {
			t.Errorf("compactTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestTokensStringBeforeAnyRequest(t *testing.T) {
	a := testApp()
	a.cfg.Model = "deepseek-chat"
	got := a.tokensString()
	if !strings.Contains(got, "deepseek-chat") {
		t.Errorf("tokens output does not mention the model:\n%s", got)
	}
	if !strings.Contains(got, "Aún no hay peticiones") {
		t.Errorf("tokens output should say there are no requests yet:\n%s", got)
	}
}

func TestTokensStringWithUsage(t *testing.T) {
	a := testApp()
	a.cfg.Model = "deepseek-chat"
	a.cfg.ReasoningEffort = "max"
	a.modelWindow = 64000
	a.addUsage(deepseek.Usage{PromptTokens: 16000, CompletionTokens: 2000, TotalTokens: 18000})

	got := a.tokensString()
	for _, want := range []string{
		"18 000", // last request total
		"16 000", // prompt
		"2 000",  // completion
		"64 000", // model window
		"25 %",   // context percentage
		"max",    // effort is shown
	} {
		if !strings.Contains(got, want) {
			t.Errorf("tokens output is missing %q:\n%s", want, got)
		}
	}
}

// Usage from several tool rounds accumulates in the session totals.
func TestUsageAccumulates(t *testing.T) {
	a := testApp()
	a.addUsage(deepseek.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110})
	a.addUsage(deepseek.Usage{PromptTokens: 200, CompletionTokens: 20, TotalTokens: 220})

	if a.tokensIn != 300 || a.tokensOut != 30 {
		t.Errorf("totals = %d/%d, want 300/30", a.tokensIn, a.tokensOut)
	}
	if a.lastUsage.TotalTokens != 220 {
		t.Errorf("lastUsage = %+v, want the most recent round", a.lastUsage)
	}
}

// The model gets the date from the prompt; otherwise it guesses the weekday.
func TestSystemPromptIncludesCurrentDate(t *testing.T) {
	a := testApp()
	prompt := a.systemPrompt()

	today := time.Now().Format("2006-01-02")
	if !strings.Contains(prompt, today) {
		t.Errorf("system prompt does not contain today's date %q:\n%s", today, prompt)
	}
	if !strings.Contains(prompt, time.Now().Format("Monday")) {
		t.Errorf("system prompt does not contain the weekday:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Fecha y hora actual") {
		t.Errorf("system prompt is missing the date label:\n%s", prompt)
	}
}

// The context mode controls how much project data goes into every request.
func TestContextModeShortensTheSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(strings.Repeat("# README largo\n", 200)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Instrucciones\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	a := testApp()

	a.cfg.Context = "full"
	a.loadProject()
	full := a.systemPrompt()

	a.cfg.Context = "light"
	a.loadProject()
	light := a.systemPrompt()
	if len(light) >= len(full) {
		t.Errorf("light prompt is not smaller: %d vs %d bytes", len(light), len(full))
	}
	if strings.Contains(light, "README largo") {
		t.Error("light mode still sends the README")
	}
	if !strings.Contains(light, "Instrucciones") {
		t.Error("light mode dropped AGENTS.md")
	}

	a.cfg.Context = "off"
	a.loadProject()
	if a.proj != nil {
		t.Error("off mode should not load the project at all")
	}
	if strings.Contains(a.systemPrompt(), "Project context") {
		t.Error("off mode still sends the project context")
	}
}

func TestContextStringReportsModeAndParts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Instrucciones\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Proyecto\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	a := testApp()
	a.cfg.Context = "full"
	a.loadProject()
	got := a.contextString()
	for _, want := range []string{"modo: full", "AGENTS.md", "README.md", "total"} {
		if !strings.Contains(got, want) {
			t.Errorf("context output is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "no incluido") {
		t.Errorf("full mode should not report omissions:\n%s", got)
	}

	a.cfg.Context = "light"
	a.loadProject()
	got = a.contextString()
	if !strings.Contains(got, "modo: light") {
		t.Errorf("mode is not reported:\n%s", got)
	}
	if !strings.Contains(got, "no incluido") {
		t.Errorf("light mode should report what it drops:\n%s", got)
	}

	a.cfg.Context = "off"
	a.loadProject()
	if got := a.contextString(); !strings.Contains(got, "modo: off") {
		t.Errorf("off mode is not reported:\n%s", got)
	}
}

func TestEstimateTokens(t *testing.T) {
	// 3 bytes per token: calibrated against a measured prompt.
	if got := estimateTokens(3000); got != "1 000" {
		t.Errorf("estimateTokens(3000) = %q, want 1 000", got)
	}
	if got := estimateTokens(0); got != "0" {
		t.Errorf("estimateTokens(0) = %q, want 0", got)
	}
}
