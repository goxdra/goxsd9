package goxsd9

import (
	"context"
	"errors"
	"io"
)

// Resolver obtains a referenced schema without interpreting its location.
// Implementations may use typed context values to carry private base state.
type Resolver interface {
	Resolve(ctx context.Context, namespaceURN, schemaLocation string) (ResolvedSource, error)
}

// ResolvedSource is one resolver-owned schema byte stream and its child context.
type ResolvedSource struct {
	ctx    context.Context
	id     SourceID
	reader io.ReadCloser
}

// NewResolvedSource constructs a resolved schema source.
func NewResolvedSource(ctx context.Context, id SourceID, reader io.ReadCloser) (ResolvedSource, error) {
	if ctx == nil {
		return ResolvedSource{}, errors.New("resolved source context is nil")
	}
	if id == "" {
		return ResolvedSource{}, errors.New("resolved source ID is empty")
	}
	if reader == nil {
		return ResolvedSource{}, errors.New("resolved source reader is nil")
	}
	return ResolvedSource{ctx: ctx, id: id, reader: reader}, nil
}

// Context returns the context to use while processing references from this source.
func (source ResolvedSource) Context() context.Context {
	return source.ctx
}

// SourceID returns the resolver-provided identity.
func (source ResolvedSource) SourceID() SourceID {
	return source.id
}

func (source ResolvedSource) stream() io.ReadCloser {
	return source.reader
}
