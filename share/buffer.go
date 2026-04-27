package share

import "sync"

const TransferBufferSize = 20480

var transferBufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, TransferBufferSize)
	},
}

func AcquireTransferBuffer() []byte {
	return transferBufferPool.Get().([]byte)
}

func ReleaseTransferBuffer(buffer []byte) {
	if cap(buffer) != TransferBufferSize {
		return
	}

	transferBufferPool.Put(buffer[:TransferBufferSize])
}
