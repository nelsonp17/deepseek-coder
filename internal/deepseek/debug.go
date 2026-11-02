package deepseek

import (
	"fmt"
	"os"
)

// Debugf logs to stderr when the DEEPSEEK_DEBUG environment variable is set.
func Debugf(format string, args ...any) {
	if os.Getenv("DEEPSEEK_DEBUG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "[deepseek] "+format+"\n", args...)
}
