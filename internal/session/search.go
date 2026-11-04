package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Hit is one match found in a stored session.
type Hit struct {
	SessionID string
	Time      time.Time
	Role      string
	Snippet   string
}

// Search bounds: a search is a convenience, not a database query.
const (
	searchMaxFiles   = 50
	searchMaxPerFile = 4096
	searchMaxHits    = 20
	searchSnippet    = 160
)

// Search looks for text in the stored sessions, newest first.
//
// The query is matched case-insensitively against the message content, not
// against the raw JSON, so what it finds is what the model actually said.
func Search(dir, query string, limit int) ([]Hit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > searchMaxHits {
		limit = searchMaxHits
	}
	needle := strings.ToLower(query)

	infos, err := List(dir)
	if err != nil {
		return nil, err
	}
	if len(infos) > searchMaxFiles {
		infos = infos[:searchMaxFiles]
	}

	var hits []Hit
	for _, info := range infos {
		if len(hits) >= limit {
			break
		}
		fileHits, err := searchFile(info.Path, info.ID, needle, limit-len(hits))
		if err != nil {
			continue // a broken log must not break the search
		}
		hits = append(hits, fileHits...)
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Time.After(hits[j].Time) })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// searchFile walks one log file.
func searchFile(path, id, needle string, limit int) ([]Hit, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var hits []Hit
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	lines := 0
	for scanner.Scan() {
		if len(hits) >= limit || lines > searchMaxPerFile {
			break
		}
		lines++
		var ev Event
		if json.Unmarshal(scanner.Bytes(), &ev) != nil {
			continue
		}
		content, ok := eventText(ev)
		if !ok || content == "" {
			continue
		}
		if idx := strings.Index(strings.ToLower(content), needle); idx >= 0 {
			hits = append(hits, Hit{
				SessionID: id,
				Time:      ev.Time,
				Role:      roleOf(ev.Type),
				Snippet:   snippetAround(content, idx, len(needle)),
			})
		}
	}
	return hits, scanner.Err()
}

// eventText returns the searchable text of an event.
func eventText(ev Event) (string, bool) {
	switch ev.Type {
	case EventUser:
		var d messageData
		if json.Unmarshal(ev.Data, &d) == nil {
			return d.Content, true
		}
	case EventAssistant:
		var d assistantData
		if json.Unmarshal(ev.Data, &d) == nil {
			return d.Content, true
		}
	}
	return "", false
}

// roleOf maps an event type to the role shown in a result.
func roleOf(eventType string) string {
	switch eventType {
	case EventUser:
		return "user"
	case EventAssistant:
		return "assistant"
	}
	return "?"
}

// snippetAround cuts a window of text around a match.
func snippetAround(text string, idx, matchLen int) string {
	start := idx - searchSnippet/3
	if start < 0 {
		start = 0
	}
	end := idx + matchLen + searchSnippet
	if end > len(text) {
		end = len(text)
	}
	// Never cut inside a UTF-8 sequence.
	for start > 0 && text[start]&0xC0 == 0x80 {
		start--
	}
	for end < len(text) && text[end]&0xC0 == 0x80 {
		end++
	}
	out := text[start:end]
	out = strings.ReplaceAll(out, "\n", " ")
	if start > 0 {
		out = "…" + out
	}
	if end < len(text) {
		out += "…"
	}
	return out
}

// SessionPath returns the file that holds a session.
func SessionPath(dir, id string) string {
	return filepath.Join(dir, id+".jsonl")
}
