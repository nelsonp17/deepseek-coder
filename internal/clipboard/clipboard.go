// Package clipboard copies text to the system clipboard.
//
// It first tries the native clipboard (Win32 API on Windows, pbcopy on macOS,
// xclip/xsel/wl-copy on Linux). When that is unavailable it falls back to the
// OSC 52 escape sequence, which most modern terminals implement and which also
// works over SSH.
package clipboard

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/atotto/clipboard"
)

// ErrEmpty is returned when there is nothing to copy.
var ErrEmpty = errors.New("nothing to copy")

var mu sync.Mutex

// Copy places text on the system clipboard.
func Copy(text string) error {
	if len(text) == 0 {
		return ErrEmpty
	}
	mu.Lock()
	defer mu.Unlock()
	if err := clipboard.WriteAll(text); err == nil {
		return nil
	}
	return osc52(text)
}

// osc52 asks the terminal to set the clipboard. It writes straight to stdout:
// the sequence produces no visible output, so it does not disturb the TUI.
func osc52(text string) error {
	payload := base64.StdEncoding.EncodeToString([]byte(text))
	_, err := fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\a", payload)
	return err
}
