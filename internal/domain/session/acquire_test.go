package session

import (
	"context"
	"main/internal/domain/image"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_LastSession(t *testing.T) {
	fakeRepo := NewFakeSessionRepo()
	fakeMeta := NewFakeMetadataRepo()
	fakeScanner := &FakeScanner{}
	fakeEventBus := &FakeEventBus{}
	topic, topicCleanup := pubsub.NewInMemoryTopic[scalar.ID]()
	defer topicCleanup()

	imageFb := image.NewFilterBuilder()
	svc, cleanupService := NewService(fakeRepo, fakeMeta, fakeScanner, fakeEventBus, zap.NewNop(), topic, "", imageFb)
	defer cleanupService()

	dirID := scalar.ToID("dir-1")

	// 1. 无会话时
	sess, release, err := svc.LastSession(context.Background(), dirID)
	require.NoError(t, err)
	assert.Nil(t, sess)
	assert.Nil(t, release)

	// 2. 创建一个会话
	sess1 := New(scalar.ToID("s1"), dirID, &shared.ImageFilters{}, 5, nil, imageFb)
	releaseCreate1, err := fakeRepo.Create(sess1)
	require.NoError(t, err)
	releaseCreate1()

	// 验证获取最后会话
	latest, releaseLatest, err := svc.LastSession(context.Background(), dirID)
	require.NoError(t, err)
	defer releaseLatest()
	assert.Equal(t, sess1.ID(), latest.ID())
}
