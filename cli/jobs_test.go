package main

import (
	"strings"
	"testing"
	"time"

	"deepseek-coder/internal/jobs"
	"deepseek-coder/internal/session"
)

func TestJobNotification(t *testing.T) {
	note := jobNotification(jobs.Result{
		ID:       3,
		Command:  "go build ./...",
		Output:   "ok\n",
		ExitCode: 0,
		Duration: 1500 * time.Millisecond,
	})
	for _, want := range []string{"job #3", "exit 0", "1.5s", "go build ./...", "ok"} {
		if !strings.Contains(note, want) {
			t.Errorf("notification is missing %q:\n%s", want, note)
		}
	}

	timeout := jobNotification(jobs.Result{ID: 1, TimedOut: true})
	if !strings.Contains(timeout, "timed out") {
		t.Errorf("timeout notification = %q", timeout)
	}
}

func TestJobNotificationTruncatesOutput(t *testing.T) {
	note := jobNotification(jobs.Result{ID: 1, Output: strings.Repeat("x", 10000)})
	if len(note) > 4200 {
		t.Errorf("notification is %d bytes, want it bounded", len(note))
	}
	if !strings.Contains(note, "truncated") {
		t.Error("the cut is not announced")
	}
}

// A background command returns at once and the result arrives later, injected
// into the session as a user message.
func TestBackgroundJobIsInjected(t *testing.T) {
	sess, err := session.Create(t.TempDir(), session.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	a := testApp()
	a.session = sess
	a.jobs = jobs.New()

	out, err := a.runShellTool(t.Context(), `{"command":"echo desde-el-job","background":true}`)
	if err != nil {
		t.Fatalf("runShellTool: %v", err)
	}
	if !strings.Contains(out, "started in the background") {
		t.Errorf("output = %q", out)
	}

	// Wait for the job to finish and drain it.
	select {
	case <-a.jobs.Done():
	case <-time.After(30 * time.Second):
		t.Fatal("the job never finished")
	}
	// The result was already consumed above; check the session instead by
	// starting a second job and injecting that one.
	_, err = a.runShellTool(t.Context(), `{"command":"echo inyectado","background":true}`)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var notes []string
	for time.Now().Before(deadline) && len(notes) == 0 {
		notes = a.injectJobResults()
		if len(notes) == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if len(notes) == 0 {
		t.Fatal("the job result was never injected")
	}
	if !strings.Contains(notes[0], "echo inyectado") {
		t.Errorf("note = %q", notes[0])
	}

	msgs := sess.Messages()
	last := msgs[len(msgs)-1]
	if last.Role != "user" || !strings.Contains(last.Content, "background job #") {
		t.Errorf("last message = %+v, want the injected note", last)
	}
}

func TestJobsStringListsRunningJobs(t *testing.T) {
	a := testApp()
	a.jobs = jobs.New()
	if got := a.jobsString(); got != a.msg.JobsNone {
		t.Errorf("empty jobs = %q", got)
	}
	a.jobs.Start("sleep 2", "", 30*time.Second)
	if got := a.jobsString(); !strings.Contains(got, "sleep 2") {
		t.Errorf("jobsString = %q", got)
	}
}
