package copier

import (
	"io"
	"sync"
)

type Copier struct {
	pool *sync.Pool
}

func NewPooledCopier(defaultBufferSize int64) *Copier {
	return &Copier{
		pool: &sync.Pool{
			New: func() interface{} {
				ret := make([]byte, defaultBufferSize)
				return &ret
			},
		},
	}
}

func NewCopier() *Copier {
	return &Copier{}
}

type writerOnly struct{ io.Writer }

type readerOnly struct{ io.Reader }

func (c *Copier) Copy(w io.Writer, r io.Reader) (int64, error) {
	if c.pool == nil {
		return io.Copy(w, r)
	}

	// Here we are blocking ReaderFrom and WriterTo optimisations to prevent fallback to io.Copy
	// https://github.com/golang/go/issues/67074 (or analogs) implementation should eliminate this hack
	// It's possible to implement trying zero-copy transfer first with fallback to io.CopyBuffer
	// but it requires some unsafe code and '-checklinkname=0' flag in go1.23 compiler.
	// For now, I don't see any reason to add unsafe code here.
	// If you have a good reason to do that, please open an issue.

	buf := c.pool.Get().(*[]byte)
	defer c.pool.Put(buf)

	return io.CopyBuffer(writerOnly{w}, readerOnly{r}, *buf)
}

func (c *Copier) CopyN(w io.Writer, r io.Reader, n int64) (int64, error) {
	written, err := c.CopyN(w, r, n)
	if written == n {
		return n, nil
	}
	if written < n && err == nil {
		// src stopped early; must have been EOF.
		err = io.EOF
	}
	return n, err
}
