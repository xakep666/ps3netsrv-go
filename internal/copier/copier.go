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

	// here we are blocking ReaderFrom and WriterTo optimisations to prevent fallback to io.Copy.

	buf := c.pool.Get().(*[]byte)
	defer c.pool.Put(buf)

	return io.CopyBuffer(writerOnly{w}, readerOnly{r}, *buf)
}
