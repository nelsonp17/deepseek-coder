package jobs

import (
	"strings"
	"testing"
	"time"
)

func TestStartReportsCompletion(t *testing.T) {
	m := New()
	id := m.Start("echo hola", "", 30*time.Second)
	if id != 1 {
		t.Errorf("first job id = %d, want 1", id)
	}

	select {
	case result := <-m.Done():
		if result.ID != id {
			t.Errorf("result id = %d, want %d", result.ID, id)
		}
		if !strings.Contains(result.Output, "hola") {
			t.Errorf("output = %q", result.Output)
		}
		if result.Err != nil {
			t.Errorf("err = %v", result.Err)
		}
		if result.Duration <= 0 {
			t.Error("duration was not measured")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the job never reported")
	}

	if running := m.Running(); len(running) != 0 {
		t.Errorf("still running: %+v", running)
	}
}

func TestIdsAreIncreasing(t *testing.T) {
	m := New()
	first := m.Start("echo 1", "", 30*time.Second)
	second := m.Start("echo 2", "", 30*time.Second)
	if second <= first {
		t.Errorf("ids are not increasing: %d then %d", first, second)
	}
	// Let them finish so the test does not leak goroutines.
	for i := 0; i < 2; i++ {
		select {
		case <-m.Done():
		case <-time.After(30 * time.Second):
			t.Fatal("timeout waiting for jobs")
		}
	}
}

func TestRunningIsVisibleWhileWorking(t *testing.T) {
	m := New()
	id := m.Start("sleep 2", "", 30*time.Second)
	if running := m.Running(); len(running) != 1 || running[0].ID != id {
		t.Errorf("Running = %+v, want the job %d", running, id)
	}
	select {
	case <-m.Done():
	case <-time.After(30 * time.Second):
		t.Fatal("timeout")
	}
}

func TestDrainIsNonBlocking(t *testing.T) {
	m := New()
	if got := m.Drain(); len(got) != 0 {
		t.Errorf("Drain on an empty manager = %+v", got)
	}
	m.Start("echo x", "", 30*time.Second)
	// Drain must never block; poll instead of sleeping a fixed amount, which is
	// what made this flaky under a loaded test run.
	deadline := time.Now().Add(30 * time.Second)
	for {
		if got := m.Drain(); len(got) == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the job never reported")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFailureIsReported(t *testing.T) {
	m := New()
	m.Start("exit 3", "", 30*time.Second)
	select {
	case result := <-m.Done():
		if result.ExitCode != 3 {
			t.Errorf("exit code = %d, want 3", result.ExitCode)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timeout")
	}
}
