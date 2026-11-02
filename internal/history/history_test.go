package history

import (
	"path/filepath"
	"testing"
)

func TestStoreCRUD(t *testing.T) {
	store, err := openAt(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	// Empty list.
	convs, err := store.ListConversations()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(convs) != 0 {
		t.Fatalf("expected no conversations, got %d", len(convs))
	}

	// Create and append messages.
	id, err := store.CreateConversation("hola mundo")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.AppendMessage(id, "user", "hola"); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := store.AppendMessage(id, "assistant", "hola!"); err != nil {
		t.Fatalf("append assistant: %v", err)
	}

	// List shows one conversation.
	convs, err = store.ListConversations()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(convs) != 1 || convs[0].Title != "hola mundo" {
		t.Fatalf("unexpected conversations: %+v", convs)
	}

	// Messages come back in order.
	msgs, err := store.GetConversation(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hola" {
		t.Fatalf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hola!" {
		t.Fatalf("unexpected second message: %+v", msgs[1])
	}

	// Delete removes everything.
	if err := store.DeleteConversation(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	msgs, err = store.GetConversation(id)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected no messages after delete, got %d", len(msgs))
	}
}

func TestDeleteMissing(t *testing.T) {
	store, err := openAt(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	if err := store.DeleteConversation(999); err == nil {
		t.Fatal("expected error deleting missing conversation")
	}
}
