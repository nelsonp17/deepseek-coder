package main

import (
	"strings"
	"testing"

	"deepseek-coder/internal/session"
	"deepseek-coder/internal/todo"
)

func listArgs(items ...string) string {
	var b strings.Builder
	b.WriteString(`{"todos":[`)
	for i, item := range items {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(item)
	}
	b.WriteString(`]}`)
	return b.String()
}

func TestTodoToolStoresTheList(t *testing.T) {
	sess, err := session.Create(t.TempDir(), session.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	a := testApp()
	a.session = sess

	out, handled, err := a.executeTodoTool(toolCall(toolTodo, listArgs(
		`{"content":"leer el archivo","status":"completed"}`,
		`{"content":"editar","status":"in_progress"}`,
	)))
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if !strings.Contains(out, "1/2") {
		t.Errorf("output lacks the progress: %q", out)
	}

	list := sess.Todos()
	if len(list) != 2 {
		t.Fatalf("stored %d tasks, want 2", len(list))
	}
	if list[1].Status != "in_progress" {
		t.Errorf("status = %q", list[1].Status)
	}
}

func TestTodoToolRejectsTwoInFlight(t *testing.T) {
	a := testApp()
	_, handled, err := a.executeTodoTool(toolCall(toolTodo, listArgs(
		`{"content":"a","status":"in_progress"}`,
		`{"content":"b","status":"in_progress"}`,
	)))
	if !handled {
		t.Fatal("the call should be handled")
	}
	if err == nil {
		t.Fatal("two in-progress tasks must be rejected")
	}
}

func TestTodoToolClearsTheList(t *testing.T) {
	sess, err := session.Create(t.TempDir(), session.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	a := testApp()
	a.session = sess
	a.executeTodoTool(toolCall(toolTodo, listArgs(`{"content":"x","status":"pending"}`)))

	out, _, err := a.executeTodoTool(toolCall(toolTodo, `{"todos":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cleared") {
		t.Errorf("output = %q", out)
	}
	if len(sess.Todos()) != 0 {
		t.Error("the list was not cleared")
	}
}

func TestTodoToolIgnoresOtherTools(t *testing.T) {
	a := testApp()
	if _, handled, _ := a.executeTodoTool(toolCall(toolRead, "{}")); handled {
		t.Error("a non-todo call must not be handled")
	}
}

func TestTodoPreview(t *testing.T) {
	preview, ok := todoPreview(listArgs(
		`{"content":"uno","status":"completed"}`,
		`{"content":"dos","status":"pending"}`,
	))
	if !ok {
		t.Fatal("expected a preview")
	}
	if !strings.Contains(preview, "1/2") || !strings.Contains(preview, "dos") {
		t.Errorf("preview = %q", preview)
	}
}

func TestTodoSurvivesAReopen(t *testing.T) {
	dir := t.TempDir()
	sess, err := session.Create(dir, session.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	id := sess.ID()
	if err := sess.AppendTodos([]todo.Item{{Content: "a", Status: todo.Pending}}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := session.Open(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if list := reopened.Todos(); len(list) != 1 {
		t.Errorf("todos after reopening = %d, want 1", len(list))
	}
}
