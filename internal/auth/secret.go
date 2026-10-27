// Package auth resolves credentials, acquires OAuth tokens, and manages their cache.
package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/stuft2/httpp/internal/config"
)

type limitedBuffer struct{ buf bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > 64<<10 {
		return 0, errors.New("credential command output exceeds 64 KiB")
	}
	return b.buf.Write(p)
}
func Resolve(ctx context.Context, s config.Secret) (string, error) {
	var value string
	if s.Env != "" {
		value = os.Getenv(s.Env)
	} else {
		if len(s.Command) == 0 {
			return "", fmt.Errorf("missing credential reference")
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, s.Command[0], s.Command[1:]...)
		var out limitedBuffer
		cmd.Stdout = &out
		// Never inherit stdin or echo subprocess stderr, which may contain secrets.
		cmd.WaitDelay = 100 * time.Millisecond
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("credential command failed or exceeded its limits")
		}
		value = out.buf.String()
	}
	value = strings.TrimRight(value, "\r\n")
	if value == "" {
		return "", fmt.Errorf("credential source returned an empty value")
	}
	return value, nil
}
