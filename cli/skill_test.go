package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"deekseep-coder/internal/deepseek"
	"deekseep-coder/internal/project"
	"deekseep-coder/internal/session"
)

// A skill reference must show the marker and what the user typed. The
// instructions themselves go to the model only: printing them makes the
// transcript look like the user pasted a manual.
func TestSkillReferenceDoesNotShowTheBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	sess, err := session.Create(t.TempDir(), session.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	a := testApp()
	a.session = sess
	client := deepseek.New(deepseek.Options{APIKey: "k", BaseURL: server.URL})
	m := newTUIModel(a, client, "deepseek-chat")

	const body = "## Persistence\r\n\r\nACTIVE EVERY RESPONSE. No revert."
	um := userMessage{
		Display: "[Skill: caveman]\n\nfull hola",
		Payload: "[Skill: caveman]\n\n" + body + "\n\nfull hola",
	}

	sent, _ := m.sendResolved(um)

	// The transcript shows the reference and the typed text.
	var shown string
	for _, e := range sent.entries {
		if e.role == "user" {
			shown += e.content
		}
	}
	if !strings.Contains(shown, "full hola") {
		t.Errorf("the typed text is missing from the transcript: %q", shown)
	}
	if strings.Contains(shown, "ACTIVE EVERY RESPONSE") {
		t.Errorf("the skill body was printed in the transcript:\n%s", shown)
	}

	// The model receives the body.
	msgs := sess.Messages()
	if len(msgs) == 0 {
		t.Fatal("the message was not stored")
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		t.Fatalf("last message role = %q", last.Role)
	}
	if !strings.Contains(last.Content, "ACTIVE EVERY RESPONSE") {
		t.Errorf("the model did not receive the skill body:\n%s", last.Content)
	}
	if !strings.Contains(last.Content, "full hola") {
		t.Errorf("the user's text was lost from the payload:\n%s", last.Content)
	}
}

// The reference resolution itself keeps display and payload apart.
func TestSkillOrToolRefSplitsDisplayAndPayload(t *testing.T) {
	dir := t.TempDir()
	path := writeSkillFile(t, dir, "caveman", "desc", "# Reglas\n\nHabla corto.\n")
	a := &app{
		msg:  testApp().msg,
		perm: testApp().perm,
		proj: &project.Project{Skills: []project.Skill{{Name: "caveman", Description: "desc", Path: path}}},
	}

	um, ok := a.skillOrToolRef("/caveman arregla el bug")
	if !ok {
		t.Fatal("not resolved")
	}
	if !strings.Contains(um.Display, "caveman") || !strings.Contains(um.Display, "arregla el bug") {
		t.Errorf("display = %q", um.Display)
	}
	if strings.Contains(um.Display, "Habla corto") {
		t.Errorf("display carries the body: %q", um.Display)
	}
	if !strings.Contains(um.Payload, "Habla corto") {
		t.Errorf("payload lost the body: %q", um.Payload)
	}
}

// The stored JSON must stay readable: a skill body is just text.
func TestSkillPayloadIsValidJSON(t *testing.T) {
	_, err := json.Marshal(userMessage{Payload: "linea\nlinea"})
	if err != nil {
		t.Fatal(err)
	}
}
