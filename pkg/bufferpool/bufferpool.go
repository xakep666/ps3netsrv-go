// bufferpool needed to reduce allocations
package bufferpool

import (
	"sync"
)

type BufferPool struct {
	pool sync.Pool
}

func NewBufferPool(baseSize int) *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				ret := make([]byte, baseSize)
				return &ret
			},
		},
	}
}

func (bp *BufferPool) Get() []byte { return *bp.pool.Get().(*[]byte) }

func (bp *BufferPool) Put(b []byte) { bp.pool.Put(&b) }
