package magick

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockCache struct {
	paths map[string]string
}

func (m *mockCache) GetPath(key string) string {
	return m.paths[key]
}

func (m *mockCache) Exists(key string) bool {
	_, ok := m.paths[key]
	return ok
}

func TestNewProcessor(t *testing.T) {
	cache := &mockCache{}
	p := NewProcessor(cache, 4)
	assert.NotNil(t, p)
	assert.NotNil(t, p.sem)
}

func TestProcessor_Semaphore(t *testing.T) {
	cache := &mockCache{}
	p := NewProcessor(cache, 4)

	ctx := context.Background()

	// Can acquire all slots
	for i := 0; i < 4; i++ {
		err := p.sem.Acquire(ctx, 1)
		assert.NoError(t, err)
	}

	// Next one should block or fail if context is canceled
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	err := p.sem.Acquire(cancelCtx, 1)
	assert.Error(t, err)

	// Release all
	for i := 0; i < 4; i++ {
		p.sem.Release(1)
	}

	// Can acquire again
	err = p.sem.Acquire(ctx, 1)
	assert.NoError(t, err)
	p.sem.Release(1)
}
