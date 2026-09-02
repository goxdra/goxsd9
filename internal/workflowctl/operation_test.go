package workflowctl

import (
	"errors"
	"testing"
)

func TestOperationBoundaryDispositionsPreserveCauses(t *testing.T) {
	sentinel := errors.New("transport failure")
	for _, test := range []struct {
		name        string
		err         error
		want        operationDisposition
		wantCause   error
		wantExitErr bool
	}{
		{name: "retryable transport", err: retryableOperation("read API", sentinel), want: operationDispositionRetryable, wantCause: sentinel},
		{name: "terminal state", err: terminalOperation("validate state", stateError("state is unsafe")), want: operationDispositionTerminal, wantExitErr: true},
		{name: "recoverable raw transport", err: retryableOperationIfRecoverable("read API", sentinel), want: operationDispositionRetryable, wantCause: sentinel},
		{name: "recoverable state", err: retryableOperationIfRecoverable("validate state", stateError("state is unsafe")), want: operationDispositionTerminal, wantExitErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := operationDispositionOf(test.err); got != test.want {
				t.Fatalf("operation disposition = %d, want %d", got, test.want)
			}
			if test.wantCause != nil && !errors.Is(test.err, test.wantCause) {
				t.Fatalf("operation error %v does not preserve cause %v", test.err, test.wantCause)
			}
			if test.wantExitErr {
				var exitErr *exitError
				if !errors.As(test.err, &exitErr) {
					t.Fatalf("operation error %v does not preserve terminal exit error", test.err)
				}
			}
		})
	}
}

func TestOperationBoundaryDoesNotReclassifyExistingDisposition(t *testing.T) {
	retryable := retryableOperation("first", errors.New("temporary"))
	if got := retryableOperationIfRecoverable("second", retryable); !errors.Is(got, retryable) {
		t.Fatalf("retryable operation was reclassified or replaced: got %v", got)
	}
	terminal := terminalOperation("first", stateError("unsafe"))
	if got := retryableOperation("second", terminal); !errors.Is(got, terminal) {
		t.Fatalf("terminal operation was reclassified or replaced: got %v", got)
	}
}
