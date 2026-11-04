package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepseek-coder/internal/config"
	"deepseek-coder/internal/deepseek"
	"deepseek-coder/internal/i18n"
	"deepseek-coder/internal/project"
)

func TestPromptLanguage(t *testing.T) {
	oldStdout := os.Stdout
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stdout = devNull
	defer func() {
		os.Stdout = oldStdout
		devNull.Close()
	}()

	tests := []struct {
		input    string
		want     i18n.Lang
		hasError bool
	}{
		{"2\n", i18n.EN, false},
		{"en\n", i18n.EN, false},
		{"english\n", i18n.EN, false},
		{"1\n", i18n.ES, false},
		{"es\n", i18n.ES, false},
		{"español\n", i18n.ES, false},
		{"garbage\n", i18n.ES, false}, // invalid input defaults to Spanish
		{"", i18n.ES, true},           // EOF with no input
	}
	for _, tt := range tests {
		got, err := promptLanguage(strings.NewReader(tt.input))
		if tt.hasError {
			if err == nil {
				t.Errorf("promptLanguage(%q): expected error, got %q", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("promptLanguage(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("promptLanguage(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPromptAPIKey(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	t.Run("reads key from stdin", func(t *testing.T) {
		oldStdout := os.Stdout
		devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		os.Stdout = devNull
		defer func() {
			os.Stdout = oldStdout
			devNull.Close()
		}()

		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdin = r
		w.WriteString("sk-test-123\n")
		w.Close()

		a := &app{msg: i18n.For(i18n.ES)}
		key, err := a.promptAPIKey()
		if err != nil {
			t.Fatalf("promptAPIKey: %v", err)
		}
		if key != "sk-test-123" {
			t.Errorf("got key %q, want %q", key, "sk-test-123")
		}
	})

	t.Run("rejects empty key", func(t *testing.T) {
		oldStdout := os.Stdout
		devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		os.Stdout = devNull
		defer func() {
			os.Stdout = oldStdout
			devNull.Close()
		}()

		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdin = r
		w.WriteString("   \n")
		w.Close()

		a := &app{msg: i18n.For(i18n.EN)}
		if _, err := a.promptAPIKey(); err == nil {
			t.Error("expected error for empty key")
		}
	})
}

func TestModelAvailable(t *testing.T) {
	models := []deepseek.Model{{ID: "deepseek-chat"}, {ID: "deepseek-reasoner"}}
	if !modelAvailable(models, "deepseek-chat") {
		t.Error("expected deepseek-chat to be available")
	}
	if modelAvailable(models, "ghost-model") {
		t.Error("expected ghost-model to be unavailable")
	}
}

func TestEnsureModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(deepseek.ModelsResponse{
			Object: "list",
			Data: []deepseek.Model{
				{ID: "deepseek-chat"},
				{ID: "deepseek-reasoner"},
			},
		})
	}))
	defer server.Close()

	client := deepseek.New(deepseek.Options{APIKey: "k", BaseURL: server.URL})

	t.Run("valid configured model is kept", func(t *testing.T) {
		a := &app{cfg: &config.Config{Model: "deepseek-reasoner"}, msg: i18n.For(i18n.ES)}
		got, err := a.ensureModel(client)
		if err != nil {
			t.Fatalf("ensureModel: %v", err)
		}
		if got != "deepseek-reasoner" {
			t.Errorf("got %q, want deepseek-reasoner", got)
		}
	})

	t.Run("stale model errors when non-interactive", func(t *testing.T) {
		old := stdinInteractive
		stdinInteractive = func() bool { return false }
		defer func() { stdinInteractive = old }()

		a := &app{cfg: &config.Config{Model: "old-model"}, msg: i18n.For(i18n.ES)}
		if _, err := a.ensureModel(client); err == nil {
			t.Fatal("expected error for stale model")
		}
	})

	t.Run("empty model errors when non-interactive", func(t *testing.T) {
		old := stdinInteractive
		stdinInteractive = func() bool { return false }
		defer func() { stdinInteractive = old }()

		a := &app{cfg: &config.Config{}, msg: i18n.For(i18n.ES)}
		if _, err := a.ensureModel(client); err == nil {
			t.Fatal("expected error for empty model")
		}
	})
}

func TestPromptModelSelection(t *testing.T) {
	config.SetConfigDir(t.TempDir())
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stdout = devNull
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
		devNull.Close()
	}()

	models := []deepseek.Model{{ID: "deepseek-chat"}, {ID: "deepseek-reasoner"}}

	t.Run("select by number", func(t *testing.T) {
		r, w, _ := os.Pipe()
		os.Stdin = r
		w.WriteString("2\n")
		w.Close()

		a := &app{cfg: &config.Config{}, msg: i18n.For(i18n.ES)}
		got, err := a.promptModelSelection(models, "")
		if err != nil {
			t.Fatalf("promptModelSelection: %v", err)
		}
		if got != "deepseek-reasoner" {
			t.Errorf("got %q, want deepseek-reasoner", got)
		}
		if a.cfg.Model != "deepseek-reasoner" {
			t.Errorf("model not persisted, cfg.Model = %q", a.cfg.Model)
		}
	})

	t.Run("select by id", func(t *testing.T) {
		r, w, _ := os.Pipe()
		os.Stdin = r
		w.WriteString("deepseek-chat\n")
		w.Close()

		a := &app{cfg: &config.Config{}, msg: i18n.For(i18n.EN)}
		got, err := a.promptModelSelection(models, "")
		if err != nil {
			t.Fatalf("promptModelSelection: %v", err)
		}
		if got != "deepseek-chat" {
			t.Errorf("got %q, want deepseek-chat", got)
		}
	})

	t.Run("invalid input then valid", func(t *testing.T) {
		r, w, _ := os.Pipe()
		os.Stdin = r
		w.WriteString("99\n1\n")
		w.Close()

		a := &app{cfg: &config.Config{}, msg: i18n.For(i18n.ES)}
		got, err := a.promptModelSelection(models, "")
		if err != nil {
			t.Fatalf("promptModelSelection: %v", err)
		}
		if got != "deepseek-chat" {
			t.Errorf("got %q, want deepseek-chat", got)
		}
	})
}

func TestConfirmTool(t *testing.T) {
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stdout = devNull
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
		devNull.Close()
	}()

	newApp := func() *app {
		return &app{
			msg:  i18n.For(i18n.ES),
			perm: &permissions{deny: map[string]bool{}},
		}
	}
	tc := deepseek.ToolCall{ID: "1", Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "echo", Arguments: "{}"}}

	interactive := func() bool { return true }

	t.Run("yes allows once", func(t *testing.T) {
		old := stdinInteractive
		stdinInteractive = interactive
		defer func() { stdinInteractive = old }()

		r, w, _ := os.Pipe()
		os.Stdin = r
		w.WriteString("y\n")
		w.Close()
		a := newApp()
		ok, err := a.confirmTool(tc)
		if err != nil || !ok {
			t.Fatalf("confirmTool = %v, %v; want true", ok, err)
		}
		if a.approvalAuto {
			t.Error("y should not change the session policy")
		}
	})

	t.Run("no denies", func(t *testing.T) {
		old := stdinInteractive
		stdinInteractive = interactive
		defer func() { stdinInteractive = old }()

		r, w, _ := os.Pipe()
		os.Stdin = r
		w.WriteString("n\n")
		w.Close()
		a := newApp()
		ok, _ := a.confirmTool(tc)
		if ok {
			t.Error("confirmTool = true, want false")
		}
	})

	// "auto" is a session policy, not a per-tool grant: it stops asking for
	// everything, which is why the UI shows a warning while it is active.
	t.Run("auto changes the session policy", func(t *testing.T) {
		old := stdinInteractive
		stdinInteractive = interactive
		defer func() { stdinInteractive = old }()

		r, w, _ := os.Pipe()
		os.Stdin = r
		w.WriteString("a\n")
		w.Close()
		a := newApp()
		ok, _ := a.confirmTool(tc)
		if !ok {
			t.Fatal("confirmTool = false, want true")
		}
		if !a.approvalAuto {
			t.Fatal("a should switch the session to auto")
		}
		// Second call must not prompt (stdin is now empty).
		ok, err := a.confirmTool(tc)
		if err != nil || !ok {
			t.Fatalf("auto policy did not allow without asking: %v, %v", ok, err)
		}
		// And it can be restored.
		if err := a.setApprovals("ask"); err != nil {
			t.Fatalf("setApprovals(ask): %v", err)
		}
		if a.approvalAuto {
			t.Error("setApprovals(ask) did not restore asking")
		}
	})

	t.Run("deny remembers", func(t *testing.T) {
		old := stdinInteractive
		stdinInteractive = interactive
		defer func() { stdinInteractive = old }()

		r, w, _ := os.Pipe()
		os.Stdin = r
		w.WriteString("d\n")
		w.Close()
		a := newApp()
		ok, _ := a.confirmTool(tc)
		if ok {
			t.Fatal("confirmTool = true, want false")
		}
		ok, _ = a.confirmTool(tc)
		if ok {
			t.Error("remembered deny failed")
		}
	})

	t.Run("non-interactive denies by default", func(t *testing.T) {
		old := stdinInteractive
		stdinInteractive = func() bool { return false }
		defer func() { stdinInteractive = old }()

		a := newApp()
		ok, err := a.confirmTool(tc)
		if err != nil || ok {
			t.Fatalf("confirmTool = %v, %v; want false (deny by default)", ok, err)
		}
	})

	// A prompt that nobody answers (EOF) must deny the call, not abort the
	// whole turn: doing that threw away the model's answer.
	t.Run("eof denies without error", func(t *testing.T) {
		old := stdinInteractive
		stdinInteractive = interactive
		defer func() { stdinInteractive = old }()

		r, w, _ := os.Pipe()
		os.Stdin = r
		w.Close() // immediate EOF
		a := newApp()
		ok, err := a.confirmTool(tc)
		if err != nil {
			t.Fatalf("confirmTool returned %v on EOF, want nil", err)
		}
		if ok {
			t.Error("an unanswered prompt must deny")
		}
	})
}

func TestLastTrigger(t *testing.T) {
	tests := []struct {
		before  string
		want    byte
		wantIdx int
	}{
		{"", 0, -1},
		{"hola", 0, -1},
		{"@", '@', 0},
		{"hola @", '@', 5},
		{"hola /models", '/', 5},
		{"@src/main", '@', 0}, // / inside the token is not a trigger
		{"hola @x /models", '/', 8},
		{"hola /models @x", '@', 13},
	}
	for _, tt := range tests {
		got, idx := lastTrigger(tt.before)
		if got != tt.want || idx != tt.wantIdx {
			t.Errorf("lastTrigger(%q) = (%q, %d), want (%q, %d)", tt.before, got, idx, tt.want, tt.wantIdx)
		}
	}
}

func TestFileCompletion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), nil, 0o644)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "util.go"), nil, 0o644)
	t.Chdir(dir)

	comp := fileCompletion("ma")
	if comp.Prefix != "@ma" {
		t.Errorf("prefix = %q, want @ma", comp.Prefix)
	}
	if len(comp.Candidates) != 2 {
		t.Fatalf("candidates = %v, want 2", comp.Candidates)
	}
	if comp.Candidates[0] != "@main.go" || comp.Candidates[1] != "@main_test.go" {
		t.Errorf("candidates = %v", comp.Candidates)
	}

	comp = fileCompletion("src/")
	want := "@src" + string(filepath.Separator) + "util.go"
	if len(comp.Candidates) != 1 || comp.Candidates[0] != want {
		t.Errorf("dir candidates = %v, want %q", comp.Candidates, want)
	}

	comp = fileCompletion("nonexistent")
	if len(comp.Candidates) != 0 {
		t.Errorf("expected no candidates for missing dir, got %v", comp.Candidates)
	}
}

// Paths with spaces are quoted so the reference stays one token, which is what
// the attachment resolver expects.
func TestFileCompletionQuotesPathsWithSpaces(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mi archivo.go"), nil, 0o644)
	t.Chdir(dir)

	comp := fileCompletion("mi")
	if len(comp.Candidates) != 1 {
		t.Fatalf("candidates = %v, want 1", comp.Candidates)
	}
	if comp.Candidates[0] != `@"mi archivo.go"` {
		t.Errorf("candidate = %q, want a quoted reference", comp.Candidates[0])
	}
}

func TestCommandCompletion(t *testing.T) {
	a := &app{
		msg:  i18n.For(i18n.ES),
		proj: &project.Project{Skills: []project.Skill{{Name: "caveman"}}},
	}
	comp := a.commandCompletion("l")
	if len(comp.Candidates) != 2 {
		t.Fatalf("candidates = %v, want [login list-models]", comp.Candidates)
	}
	if comp.Candidates[0] != "/list-models" || comp.Candidates[1] != "/login" {
		t.Errorf("candidates = %v", comp.Candidates)
	}

	comp = a.commandCompletion("cav")
	if len(comp.Candidates) != 1 || comp.Candidates[0] != "/caveman" {
		t.Errorf("skill candidates = %v", comp.Candidates)
	}
}

func TestSkillOrToolRef(t *testing.T) {
	a := &app{
		msg:  i18n.For(i18n.ES),
		proj: &project.Project{Skills: []project.Skill{{Name: "caveman"}}},
	}
	ref, ok := a.skillOrToolRef("/caveman haz algo")
	if !ok {
		t.Fatal("expected skill reference")
	}
	if !strings.Contains(ref.Payload, "caveman") || !strings.Contains(ref.Payload, "haz algo") {
		t.Errorf("payload = %q", ref.Payload)
	}
	if !strings.Contains(ref.Display, "haz algo") {
		t.Errorf("display lost the typed text: %q", ref.Display)
	}
	if strings.Contains(ref.Display, "Persistence") {
		t.Errorf("the skill body leaked into the display: %q", ref.Display)
	}

	if _, ok := a.skillOrToolRef("/no-existe"); ok {
		t.Error("unknown reference should not match")
	}
}

func TestShortTitle(t *testing.T) {
	if got := shortTitle("hola mundo"); got != "hola mundo" {
		t.Errorf("shortTitle short = %q", got)
	}
	long := strings.Repeat("a", 100)
	if got := shortTitle(long); len([]rune(got)) != 61 { // 60 runes + ellipsis
		t.Errorf("shortTitle long length = %d runes, want 61", len([]rune(got)))
	}
}
