package graphql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryResolverMetaLoadLatencyThreshold(t *testing.T) {
	t.Run("disabled is null", func(t *testing.T) {
		r := &queryResolver{Resolver: &Resolver{loadLatencyThresholdMs: 0}}

		meta, err := r.Meta(context.Background())
		require.NoError(t, err)
		assert.Nil(t, meta.LoadLatencyThresholdMs, "关闭采集时客户端应拿到 null，而不是 0")
	})

	t.Run("enabled exposes threshold", func(t *testing.T) {
		r := &queryResolver{Resolver: &Resolver{loadLatencyThresholdMs: 750}}

		meta, err := r.Meta(context.Background())
		require.NoError(t, err)
		require.NotNil(t, meta.LoadLatencyThresholdMs)
		assert.Equal(t, 750, *meta.LoadLatencyThresholdMs)
	})
}
