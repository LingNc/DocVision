package luadraw

import "sync"

// Keep a bounded tail even when a broken TeX/Lua program prints forever.
type boundedOutput struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	const limit = 65536
	if len(p) >= limit {
		b.data = append(b.data[:0], p[len(p)-limit:]...)
		b.truncated = true
		return n, nil
	}
	if extra := len(b.data) + len(p) - limit; extra > 0 {
		copy(b.data, b.data[extra:])
		b.data = b.data[:len(b.data)-extra]
		b.truncated = true
	}
	b.data = append(b.data, p...)
	return n, nil
}
func (b *boundedOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		return "...[log truncated]...\n" + string(b.data)
	}
	return string(b.data)
}
