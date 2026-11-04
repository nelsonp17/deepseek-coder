package main

import (
	"strings"
	"testing"

	"deepseek-coder/internal/config"
	"deepseek-coder/internal/deepseek"
)

func TestPlanModeBlocksWrites(t *testing.T) {
	a := testApp()
	a.planMode = true

	for _, name := range []string{toolWrite, toolEdit, shellToolName} {
		denial, blocked := a.sandboxDenial(toolCall(name, "{}"))
		if !blocked {
			t.Errorf("%s should be blocked in plan mode", name)
			continue
		}
		if !strings.Contains(denial, "plan mode") {
			t.Errorf("denial for %s does not explain plan mode: %q", name, denial)
		}
		if !strings.Contains(denial, "exit_plan_mode") {
			t.Errorf("denial for %s does not say what to do: %q", name, denial)
		}
	}
	// Reading is the point.
	for _, name := range []string{toolRead, toolGlob, toolGrep} {
		if _, blocked := a.sandboxDenial(toolCall(name, "{}")); blocked {
			t.Errorf("%s must be allowed in plan mode", name)
		}
	}
	// Answering a question is not writing.
	if _, blocked := a.sandboxDenial(toolCall(toolQuestion, "{}")); blocked {
		t.Error("ask_user_question must work in plan mode")
	}
}

func TestPlanToolOnlyExistsInPlanMode(t *testing.T) {
	a := testApp()
	a.cfg = &config.Config{Shell: "off"}

	if hasTool(a.allTools(), toolPlan) {
		t.Error("exit_plan_mode must not be offered outside plan mode")
	}
	a.planMode = true
	if !hasTool(a.allTools(), toolPlan) {
		t.Error("exit_plan_mode must be offered in plan mode")
	}
	if !hasTool(a.allTools(), toolQuestion) {
		t.Error("ask_user_question must always be offered")
	}
	if !hasTool(a.allTools(), toolTodo) {
		t.Error("todo_write must always be offered")
	}
}

func hasTool(tools []deepseek.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Function.Name == name {
			return true
		}
	}
	return false
}

func TestExitPlanModeTurnsPlanModeOff(t *testing.T) {
	a := testApp()
	a.planMode = true

	out, handled, err := a.executePlanTool(toolCall(toolPlan, `{"plan":"1. leer\n2. editar"}`))
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if a.planMode {
		t.Error("plan mode is still on")
	}
	if !strings.Contains(out, "approved") {
		t.Errorf("output = %q", out)
	}
}

func TestExitPlanModeRejectsAnEmptyPlan(t *testing.T) {
	a := testApp()
	a.planMode = true
	if _, _, err := a.executePlanTool(toolCall(toolPlan, `{"plan":"  "}`)); err == nil {
		t.Error("an empty plan must be rejected")
	}
	if !a.planMode {
		t.Error("a rejected plan must not leave plan mode")
	}
}

func TestPlanPreview(t *testing.T) {
	preview, ok := planPreview(`{"plan":"cambiar el mux"}`)
	if !ok || !strings.Contains(preview, "cambiar el mux") || !strings.Contains(preview, "PLAN") {
		t.Errorf("preview = %q, %v", preview, ok)
	}
	if _, ok := planPreview(`{"plan":""}`); ok {
		t.Error("an empty plan has no preview")
	}
}

// Approving a plan is always a human decision, even in auto mode.
func TestExitPlanModeAlwaysAsks(t *testing.T) {
	a := testApp()
	a.approvalAuto = true
	if !a.callNeedsApproval(toolCall(toolPlan, "{}")) {
		t.Error("exit_plan_mode must ask even with approval auto")
	}
}

func TestQuestionWithoutUIAsksInText(t *testing.T) {
	a := testApp() // no askUser: line mode
	out, handled, err := a.executeQuestionTool(toolCall(toolQuestion,
		`{"question":"¿Qué hago?","options":[{"label":"A"},{"label":"B"}]}`))
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if !strings.Contains(out, "plain text") {
		t.Errorf("output = %q, want it to tell the model to ask in text", out)
	}
}

func TestQuestionReturnsTheAnswer(t *testing.T) {
	a := testApp()
	var asked string
	a.askUser = func(question string, options []string, multi bool) ([]string, error) {
		asked = question + "|" + strings.Join(options, ",")
		return []string{"Postgres"}, nil
	}
	out, _, err := a.executeQuestionTool(toolCall(toolQuestion,
		`{"question":"¿Qué base?","options":[{"label":"SQLite"},{"label":"Postgres"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if asked != "¿Qué base?|SQLite,Postgres" {
		t.Errorf("asked = %q", asked)
	}
	if !strings.Contains(out, "Postgres") {
		t.Errorf("output = %q", out)
	}
}

func TestQuestionDismissed(t *testing.T) {
	a := testApp()
	a.askUser = func(string, []string, bool) ([]string, error) { return nil, nil }
	out, _, err := a.executeQuestionTool(toolCall(toolQuestion,
		`{"question":"¿?","options":[{"label":"A"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dismissed") {
		t.Errorf("output = %q", out)
	}
}

func TestQuestionValidation(t *testing.T) {
	a := testApp()
	a.askUser = func(string, []string, bool) ([]string, error) { return []string{"A"}, nil }

	cases := []string{
		`{"question":"","options":[{"label":"A"}]}`,
		`{"question":"¿?","options":[]}`,
		`{"question":"¿?","options":[{"label":"1"},{"label":"2"},{"label":"3"},{"label":"4"},{"label":"5"},{"label":"6"},{"label":"7"}]}`,
	}
	for _, args := range cases {
		if _, _, err := a.executeQuestionTool(toolCall(toolQuestion, args)); err == nil {
			t.Errorf("args %s should be rejected", args)
		}
	}
}
