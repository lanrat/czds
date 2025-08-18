package czds

import (
	"context"
	"io"
	"strings"
)

// slice2LowerMap converts a slice of strings to a map with lowercase keys for fast lookup.
func slice2LowerMap(array []string) map[string]bool {
	out := make(map[string]bool)

	for _, s := range array {
		out[strings.ToLower(s)] = true
	}

	return out
}

// context aware reader
type readerCtx struct {
	ctx context.Context
	r   io.Reader
}

func (r *readerCtx) Read(p []byte) (n int, err error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// NewReaderCtx gets a context-aware io.Reader.
func NewReaderCtx(ctx context.Context, r io.Reader) io.Reader {
	return &readerCtx{ctx: ctx, r: r}
}
