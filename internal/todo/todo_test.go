package todo

import (
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	ok := List{
		{Content: "leer el archivo", Status: Completed},
		{Content: "editar la función", Status: InProgress},
		{Content: "ejecutar tests", Status: Pending},
	}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid list rejected: %v", err)
	}

	cases := []struct {
		name string
		list List
		want error
	}{
		{"empty content", List{{Content: "  ", Status: Pending}}, ErrEmptyContent},
		{"bad status", List{{Content: "x", Status: "doing"}}, ErrBadStatus},
		{"two in flight", List{
			{Content: "a", Status: InProgress},
			{Content: "b", Status: InProgress},
		}, ErrTwoInFlight},
	}
	for _, c := range cases {
		if err := c.list.Validate(); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}

	tooMany := make(List, 51)
	for i := range tooMany {
		tooMany[i] = Item{Content: "x", Status: Pending}
	}
	if err := tooMany.Validate(); !errors.Is(err, ErrTooMany) {
		t.Errorf("51 tasks: err = %v, want ErrTooMany", err)
	}
}

func TestCountsAndProgress(t *testing.T) {
	l := List{
		{Content: "a", Status: Completed},
		{Content: "b", Status: Completed},
		{Content: "c", Status: InProgress},
		{Content: "d", Status: Pending},
	}
	if done, total := l.Counts(); done != 2 || total != 4 {
		t.Errorf("Counts = %d/%d, want 2/4", done, total)
	}
	if got := l.Progress(); got != "2/4" {
		t.Errorf("Progress = %q, want 2/4", got)
	}
}

func TestRender(t *testing.T) {
	l := List{
		{Content: "uno", Status: Completed},
		{Content: "dos", Status: InProgress},
		{Content: "tres", Status: Pending},
	}
	out := l.Render()
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "●") || !strings.Contains(lines[1], "◐") || !strings.HasPrefix(lines[2], "○") {
		t.Errorf("icons are wrong:\n%s", out)
	}
	if !strings.Contains(lines[1], "2. dos") {
		t.Errorf("numbering is wrong: %q", lines[1])
	}
}

func TestRenderEmpty(t *testing.T) {
	if got := (List{}).Render(); got != "" {
		t.Errorf("Render(empty) = %q, want empty", got)
	}
}
