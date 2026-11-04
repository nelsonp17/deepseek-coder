// Package jobs runs shell commands in the background.
//
// A long build or test run should not block the conversation: the model starts
// it, gets an id, and keeps talking. When the job finishes the result is handed
// back through a channel, so the notification is pushed instead of polled.
package jobs

import (
	"context"
	"sync"
	"time"

	"deepseek-coder/internal/shell"
)

// Result is what a finished job produced.
type Result struct {
	ID       int
	Command  string
	Output   string
	ExitCode int
	TimedOut bool
	Duration time.Duration
	Err      error
}

// Job is one command running in the background.
type Job struct {
	ID      int
	Command string
	Started time.Time
}

// Manager starts jobs and reports when they finish.
type Manager struct {
	mu   sync.Mutex
	next int
	jobs map[int]Job
	done chan Result
}

// New creates an empty manager.
func New() *Manager {
	return &Manager{jobs: make(map[int]Job), done: make(chan Result, 64)}
}

// Start runs a command in the background and returns its id.
func (m *Manager) Start(command, dir string, timeout time.Duration) int {
	m.mu.Lock()
	m.next++
	id := m.next
	m.jobs[id] = Job{ID: id, Command: command, Started: time.Now()}
	m.mu.Unlock()

	go func() {
		result, err := shell.Run(context.Background(), shell.Options{Command: command, Dir: dir, Timeout: timeout})

		m.mu.Lock()
		job := m.jobs[id]
		delete(m.jobs, id)
		m.mu.Unlock()

		m.done <- Result{
			ID:       id,
			Command:  command,
			Output:   result.Output,
			ExitCode: result.ExitCode,
			TimedOut: result.TimedOut,
			Duration: time.Since(job.Started),
			Err:      err,
		}
	}()
	return id
}

// Running returns a snapshot of the jobs still running.
func (m *Manager) Running() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		out = append(out, job)
	}
	// Stable order by id.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].ID < out[j-1].ID; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Done is the channel that reports finished jobs, in completion order.
func (m *Manager) Done() <-chan Result { return m.done }

// Drain takes every finished job without blocking.
func (m *Manager) Drain() []Result {
	var out []Result
	for {
		select {
		case result := <-m.done:
			out = append(out, result)
		default:
			return out
		}
	}
}
