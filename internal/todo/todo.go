// Package todo models the task list an agent keeps while working on something
// that takes more than one step.
//
// The list is not decoration: it is written to the session log, so it survives a
// resume, and it is shown above the input box so the user can see what the model
// thinks it is doing.
package todo

import (
	"errors"
	"fmt"
	"strings"
)

// Status is where a task stands.
type Status string

// The three states a task can be in.
const (
	Pending    Status = "pending"
	InProgress Status = "in_progress"
	Completed  Status = "completed"
)

// Item is one task.
type Item struct {
	Content string `json:"content"`
	Status  Status `json:"status"`
}

// List is an ordered set of tasks.
type List []Item

// Errors reported by Validate.
var (
	ErrEmptyContent = errors.New("a task needs content")
	ErrBadStatus    = errors.New("status must be pending, in_progress or completed")
	ErrTwoInFlight  = errors.New("only one task can be in_progress at a time")
	ErrTooMany      = errors.New("the list is limited to 50 tasks")
)

// Validate checks a list before it is stored.
//
// It enforces a single in-progress task on purpose: an agent that marks three
// things as "doing" is usually doing none of them.
func (l List) Validate() error {
	if len(l) > 50 {
		return ErrTooMany
	}
	inFlight := 0
	for i, item := range l {
		if strings.TrimSpace(item.Content) == "" {
			return fmt.Errorf("task %d: %w", i+1, ErrEmptyContent)
		}
		switch item.Status {
		case Pending, InProgress, Completed:
		default:
			return fmt.Errorf("task %d: %w", i+1, ErrBadStatus)
		}
		if item.Status == InProgress {
			inFlight++
		}
	}
	if inFlight > 1 {
		return ErrTwoInFlight
	}
	return nil
}

// Counts returns how many tasks are done and how many there are.
func (l List) Counts() (done, total int) {
	for _, item := range l {
		if item.Status == Completed {
			done++
		}
	}
	return done, len(l)
}

// Icon is the glyph that represents a status.
func (s Status) Icon() string {
	switch s {
	case Completed:
		return "●"
	case InProgress:
		return "◐"
	default:
		return "○"
	}
}

// Render writes the list as plain text, one task per line.
func (l List) Render() string {
	var b strings.Builder
	for i, item := range l {
		fmt.Fprintf(&b, "%s %d. %s\n", item.Status.Icon(), i+1, item.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}

// Progress returns a compact "2/5" summary.
func (l List) Progress() string {
	done, total := l.Counts()
	return fmt.Sprintf("%d/%d", done, total)
}
