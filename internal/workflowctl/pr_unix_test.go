//go:build unix

package workflowctl

import (
	"io"
	"path/filepath"
	"syscall"
	"testing"
)

func TestFinishRejectsFIFOWithoutOpeningIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("create summary FIFO: %v", err)
	}
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		t.Fatalf("FIFO summary executed %s %v", name, args)
		return "", nil
	}}
	if err := application.runPR([]string{"finish", "35", "--summary-file", path}); err == nil {
		t.Fatal("FIFO summary was accepted")
	}
}
