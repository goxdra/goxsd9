package workflowctl

import (
	"errors"
	"fmt"
)

// operationDisposition identifies whether a failed operation can be retried
// at the same workflow boundary.  Callers use this typed value instead of
// interpreting command stderr, which keeps transport and API failures
// retryable without confusing them with an authenticated terminal decision.
type operationDisposition uint8

const (
	operationDispositionUnknown operationDisposition = iota
	operationDispositionRetryable
	operationDispositionTerminal
)

type operationBoundaryError struct {
	operation   string
	disposition operationDisposition
	err         error
}

func (e *operationBoundaryError) Error() string {
	return fmt.Sprintf("%s operation (%s): %v", e.operation, operationDispositionName(e.disposition), e.err)
}

func (e *operationBoundaryError) Unwrap() error {
	return e.err
}

func retryableOperation(operation string, err error) error {
	if err == nil {
		return nil
	}
	if operationDispositionOf(err) != operationDispositionUnknown {
		return err
	}
	return &operationBoundaryError{operation: operation, disposition: operationDispositionRetryable, err: err}
}

func retryableOperationIfRecoverable(operation string, err error) error {
	if err == nil {
		return nil
	}
	if operationDispositionOf(err) != operationDispositionUnknown {
		return err
	}
	var terminal *exitError
	if errors.As(err, &terminal) {
		return terminalOperation(operation, err)
	}
	return retryableOperation(operation, err)
}

func terminalOperation(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &operationBoundaryError{operation: operation, disposition: operationDispositionTerminal, err: err}
}

func operationDispositionOf(err error) operationDisposition {
	var boundary *operationBoundaryError
	if !errors.As(err, &boundary) {
		return operationDispositionUnknown
	}
	return boundary.disposition
}

func operationDispositionName(disposition operationDisposition) string {
	switch disposition {
	case operationDispositionUnknown:
		return "unknown"
	case operationDispositionRetryable:
		return "retryable"
	case operationDispositionTerminal:
		return "terminal"
	}
	return "unknown"
}
