package goxsd9

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestResolvedSource(t *testing.T) {
	reader := io.NopCloser(strings.NewReader("schema"))
	source, err := NewResolvedSource(context.Background(), "urn:test", reader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	if got, want := source.SourceID(), SourceID("urn:test"); got != want {
		t.Fatalf("SourceID() = %q, want %q", got, want)
	}
	data, err := io.ReadAll(source.stream())
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if err := source.stream().Close(); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	if got, want := string(data), "schema"; got != want {
		t.Fatalf("stream = %q, want %q", got, want)
	}
}
