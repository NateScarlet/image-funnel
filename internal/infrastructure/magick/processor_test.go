package magick

import (
	"context"
	"io"
	"testing"

	appimage "main/internal/application/image"

	"github.com/stretchr/testify/assert"
)

type mockCache struct {
}

func (m *mockCache) Open(ctx context.Context, key string) (appimage.File, error) {
	return nil, nil
}

func (m *mockCache) Save(ctx context.Context, key string, r io.Reader) error {
	return nil
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
