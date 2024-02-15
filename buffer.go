package actress

import (
	"bytes"
	"sync"
)

type Buffer struct {
	buffer bytes.Buffer
	mu     sync.Mutex
}

func NewBuffer() *Buffer {
	b := Buffer{}
	return &b
}

func (bu *Buffer) Read(p []byte) (int, error) {
	bu.mu.Lock()
	defer bu.mu.Unlock()
	return bu.buffer.Read(p)
}

func (bu *Buffer) Write(b []byte) (int, error) {
	bu.mu.Lock()
	defer bu.mu.Unlock()
	return bu.buffer.Write(b)
}
