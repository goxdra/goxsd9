package workflowctl

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestRunFuzzRunningStagesExpireAtConfiguredBounds(t *testing.T) {
	for _, test := range []struct {
		name  string
		stage int
		bound time.Duration
	}{
		{name: "build", stage: 1, bound: 3 * time.Minute},
		{name: "corpus and seed replay", stage: 2, bound: 30 * time.Second},
		{name: "fuzz execution", stage: 3, bound: 30*time.Second + 250*time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			runFuzzRunningStageDeadlineCase(t, test.name, test.stage, test.bound)
		})
	}
}

func runFuzzRunningStageDeadlineCase(t *testing.T, phase string, stopStage int, stopBound time.Duration) {
	t.Helper()
	root := newFuzzFixture(t)
	var report bytes.Buffer
	var runErr error
	stage := 0
	fakeElapsed := time.Duration(0)
	synctest.Test(t, func(t *testing.T) {
		application := fuzzTestApplication(t, root, &report)
		application.executeCommandWithContextAndEnv = runningFuzzDeadlineExecutor(t,
			&stage, &fakeElapsed, stopStage, stopBound)
		runErr = application.runFuzz([]string{
			"--package", ".", "--target", "FuzzFixture", "--duration", "250ms",
		})
	})
	if stage != stopStage || fakeElapsed != stopBound+time.Nanosecond {
		t.Fatalf("running %s reached stage %d after fake time %s, want stop at %d after %s",
			phase, stage, fakeElapsed, stopStage, stopBound+time.Nanosecond)
	}
	var diagnostic *fuzzDiagnostic
	if !errors.As(runErr, &diagnostic) || diagnostic.code != fuzzTimeoutCode ||
		!errors.Is(runErr, context.DeadlineExceeded) {
		t.Fatalf("running %s error = %v, want WFZ1004 preserving deadline cause", phase, runErr)
	}
	if !strings.Contains(runErr.Error(), phase+" exceeded the "+stopBound.String()+" phase bound") ||
		!strings.Contains(runErr.Error(), "running stage output") {
		t.Fatalf("running %s diagnostic lost phase, bound, or child output: %v", phase, runErr)
	}
	if !strings.Contains(report.String(), "result: timeout\n") ||
		!strings.Contains(report.String(), "duration: 250ms\nworkers: 1\noffline: true\n") {
		t.Fatalf("running %s report = %q", phase, report.String())
	}
	source := assertFuzzEvidenceSource(t, report.String())
	if err := os.RemoveAll(filepath.Dir(source)); err != nil {
		t.Fatalf("remove retained %s evidence: %v", phase, err)
	}
}

func runningFuzzDeadlineExecutor(t *testing.T, stage *int, fakeElapsed *time.Duration,
	stopStage int, stopBound time.Duration,
) commandContextEnvironmentExecutor {
	t.Helper()
	return func(ctx context.Context, _ string, _ []string, _ io.Reader, _ string,
		_ ...string,
	) (string, error) {
		*stage++
		wantBounds := []time.Duration{3 * time.Minute, 30 * time.Second, 30*time.Second + 250*time.Millisecond}
		if *stage > len(wantBounds) {
			t.Fatalf("unexpected stage %d", *stage)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) != wantBounds[*stage-1] {
			t.Fatalf("stage %d deadline = %s (set %t), want %s", *stage,
				time.Until(deadline), ok, wantBounds[*stage-1])
		}
		if *stage != stopStage {
			return "", nil
		}
		start := time.Now()
		time.Sleep(stopBound + time.Nanosecond)
		*fakeElapsed = time.Since(start)
		return "running stage output", ctx.Err()
	}
}
