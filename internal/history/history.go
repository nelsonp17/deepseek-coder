package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS conversations (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	title      TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	role            TEXT NOT NULL,
	content         TEXT NOT NULL,
	created_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id);
`

// Conversation is a stored chat session.
type Conversation struct {
	ID        int64
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message is a single stored message.
type Message struct {
	ID        int64
	Role      string
	Content   string
	CreatedAt time.Time
}

// Store persists conversations in a SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the history database in the user's data
// directory.
func Open() (*Store, error) {
	appDir := filepath.Join(userDataDir(), "deekseep-coder")
	return openAt(filepath.Join(appDir, "history.db"))
}

// openAt opens the database at a specific path (used by tests too).
func openAt(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, err
	}
	// _pragma enables foreign keys (ON DELETE CASCADE) per connection.
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// userDataDir returns the platform data directory:
// Windows %LOCALAPPDATA%, macOS ~/Library/Application Support, Linux
// $XDG_DATA_HOME or ~/.local/share.
func userDataDir() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support")
	}
	return filepath.Join(home, ".local", "share")
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// CreateConversation inserts a new conversation and returns its ID.
func (s *Store) CreateConversation(title string) (int64, error) {
	now := time.Now().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO conversations (title, created_at, updated_at) VALUES (?, ?, ?)`,
		title, now, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AppendMessage stores a message and bumps the conversation's updated_at.
func (s *Store) AppendMessage(convID int64, role, content string) error {
	now := time.Now().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO messages (conversation_id, role, content, created_at) VALUES (?, ?, ?, ?)`,
		convID, role, content, now,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE conversations SET updated_at = ? WHERE id = ?`, now, convID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ListConversations returns all conversations, most recently updated first.
func (s *Store) ListConversations() ([]Conversation, error) {
	rows, err := s.db.Query(
		`SELECT id, title, created_at, updated_at FROM conversations ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Conversation
	for rows.Next() {
		var c Conversation
		var created, updated string
		if err := rows.Scan(&c.ID, &c.Title, &created, &updated); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, created)
		c.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetConversation returns the messages of a conversation in order.
func (s *Store) GetConversation(id int64) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, role, content, created_at FROM messages WHERE conversation_id = ? ORDER BY id`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &created); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteConversation removes a conversation and its messages.
func (s *Store) DeleteConversation(id int64) error {
	res, err := s.db.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("conversation %d not found", id)
	}
	return nil
}
