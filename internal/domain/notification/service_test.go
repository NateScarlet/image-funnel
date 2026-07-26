package notification

import (
	"context"
	"iter"
	"testing"

	"main/internal/apperror"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// mockRepo 实现 Repository，内存储存
type mockRepo struct {
	notifs map[string]*Notification
}

func newMockRepo() *mockRepo {
	return &mockRepo{notifs: make(map[string]*Notification)}
}

func (r *mockRepo) Save(ctx context.Context, n *Notification) (bool, error) {
	tag := n.Tag()
	_, exists := r.notifs[tag]
	r.notifs[tag] = n
	return !exists, nil
}

func (r *mockRepo) Get(ctx context.Context, id string) (*Notification, error) {
	for _, n := range r.notifs {
		if n.ID().String() == id {
			return n, nil
		}
	}
	return nil, apperror.NewErrDocumentNotFound(scalar.ID{})
}

func (r *mockRepo) GetByTag(ctx context.Context, tag string) (*Notification, error) {
	n, ok := r.notifs[tag]
	if !ok {
		return nil, apperror.NewErrDocumentNotFound(scalar.ID{})
	}
	return n, nil
}

func (r *mockRepo) Find(ctx context.Context, opts ...FindOption) iter.Seq2[*Notification, error] {
	return func(yield func(*Notification, error) bool) {}
}

func (r *mockRepo) Channels(ctx context.Context) iter.Seq2[*ChannelStats, error] {
	return func(yield func(*ChannelStats, error) bool) {}
}

type mockTopic struct {
	published []*shared.NotificationChangedEventDTO
}

func (t *mockTopic) Publish(ctx context.Context, event *shared.NotificationChangedEventDTO, _ ...pubsub.PublishOption) error {
	t.published = append(t.published, event)
	return nil
}

func (t *mockTopic) Subscribe(ctx context.Context) iter.Seq2[*shared.NotificationChangedEventDTO, error] {
	return func(yield func(*shared.NotificationChangedEventDTO, error) bool) {}
}

func TestServiceSend_WithoutTag_GeneratesUUID(t *testing.T) {
	svc := NewService(newMockRepo(), &Factory{}, &mockTopic{})
	result, err := svc.Send(context.Background(), "test-channel", "Test Title", shared.WithBody("hello"))
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.DidCreate())

	idStr := result.ID().String()
	assert.Contains(t, idStr, "notify:")
	tagPart := idStr[len("notify:"):]
	_, err = uuid.Parse(tagPart)
	assert.NoError(t, err, "should generate valid UUID tag")
}

func TestServiceSend_WithTag(t *testing.T) {
	svc := NewService(newMockRepo(), &Factory{}, &mockTopic{})
	tag := uuid.NewString()
	result, err := svc.Send(context.Background(), "test-channel", "Test Title", shared.WithTag(tag))
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.DidCreate())
}

func TestServiceSend_WithSuffixTag(t *testing.T) {
	svc := NewService(newMockRepo(), &Factory{}, &mockTopic{})
	id := uuid.NewString()
	tag := id + ".progress"
	result, err := svc.Send(context.Background(), "test-channel", "Test Title", shared.WithTag(tag))
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.DidCreate())
}

func TestServiceSend_UpdateExisting(t *testing.T) {
	svc := NewService(newMockRepo(), &Factory{}, &mockTopic{})
	tag := uuid.NewString()

	result1, err := svc.Send(context.Background(), "test-channel", "Test Title", shared.WithTag(tag))
	assert.NoError(t, err)
	assert.True(t, result1.DidCreate())

	result2, err := svc.Send(context.Background(), "test-channel", "Updated Title", shared.WithTag(tag))
	assert.NoError(t, err)
	assert.False(t, result2.DidCreate())
}

func TestServiceSend_ChannelConflict(t *testing.T) {
	svc := NewService(newMockRepo(), &Factory{}, &mockTopic{})
	tag := uuid.NewString()

	_, err := svc.Send(context.Background(), "channel-a", "Test Title", shared.WithTag(tag))
	assert.NoError(t, err)

	_, err = svc.Send(context.Background(), "channel-b", "Test Title", shared.WithTag(tag))
	assert.Error(t, err)
	assert.Equal(t, "CHANNEL_CONFLICT", apperror.ErrCode(err))
}
