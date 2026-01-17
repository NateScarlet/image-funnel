package pubsub_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"main/internal/pubsub"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryTopic(t *testing.T) {
	var ctx = context.Background()
	t.Run("should preserve order", func(t *testing.T) {
		t.Parallel()
		var topic, cleanup = pubsub.NewInMemoryTopic[int]()
		defer cleanup()
		var seq = []int{0, 1, 2, 3, 4, 5}
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		var ack = make(chan struct{})
		go func() {
			time.Sleep(10 * time.Millisecond)
			for _, i := range seq {
				var err = topic.Publish(ctx, i)
				if errors.Is(err, context.Canceled) {
					return
				}
				require.NoError(t, err)
				select {
				case <-ctx.Done():
					return
				case ack <- struct{}{}:
				}
			}
		}()
		var index int
		for i, err := range topic.Subscribe(ctx) {
			t.Logf("receive %d", i)
			<-ack
			require.NoError(t, err)
			require.Equal(t, seq[index], i)
			index++
			if index == len(seq) {
				break
			}
		}
	})
	t.Run("should return error after topic disposed", func(t *testing.T) {
		t.Parallel()
		var topic, cleanup = pubsub.NewInMemoryTopic[int](pubsub.InMemoryTopicWithPublishBuffer(0))
		cleanup()
		assert.ErrorIs(t, topic.Publish(ctx, 1), pubsub.ErrTopicDisposed)
		for _, err := range topic.Subscribe(ctx) {
			assert.ErrorIs(t, err, pubsub.ErrTopicDisposed)
			break
		}
	})
	t.Run("should stop on context cancel", func(t *testing.T) {
		t.Parallel()
		var topic, cleanup = pubsub.NewInMemoryTopic[int]()
		defer cleanup()
		var ctx, cancel = context.WithCancel(ctx)
		time.AfterFunc(time.Second, cancel)
		for _, err := range topic.Subscribe(ctx) {
			assert.ErrorIs(t, err, context.Canceled)
		}
	})
	t.Run("should return ignorable error on undelivered events", func(t *testing.T) {
		t.Parallel()
		var topic, cleanup = pubsub.NewInMemoryTopic[int](
			pubsub.InMemoryTopicOptionCapacity(3),
			pubsub.InMemoryTopicWithPublishBuffer(0),
			pubsub.InMemoryTopicWithMaxBatchWait(0),
		)
		defer cleanup()
		var ack = make(chan struct{})
		const messageCount = 10
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		go func() {
			time.Sleep(10 * time.Millisecond)
			for i := range messageCount {
				var err = topic.Publish(ctx, i)
				require.NoError(t, err)
				select {
				case <-ctx.Done():
					return
				case ack <- struct{}{}:
				}
			}
		}()
		var receiveCount int
		for i, err := range topic.Subscribe(ctx) {
			t.Logf("receive %d", i)
			receiveCount++
			if i < 4 {
				<-ack
			} else if i == 4 {
				for range messageCount - i {
					<-ack
				}
			}
			if i == 7 {
				assert.Error(t, err)
				assert.ErrorIs(t, err, pubsub.ErrUndeliveredEvents)
			} else {
				require.NoError(t, err)
			}
			if i == messageCount-1 {
				break
			}
		}
		assert.Equal(t, messageCount-2, receiveCount)
	})
}

func TestInMemoryTopic_BasicPublishSubscribe(t *testing.T) {
	t.Parallel()
	topic, cleanup := pubsub.NewInMemoryTopic[string]()
	defer cleanup()
	ctx := context.Background()

	// Subscribe
	result := make(chan string, 1)
	go func() {
		for val, err := range topic.Subscribe(ctx) {
			if err != nil {
				return
			}
			result <- val
			return
		}
	}()

	// Ensure subscriber is ready
	time.Sleep(10 * time.Millisecond)

	// Publish and verify
	err := topic.Publish(ctx, "test")
	require.NoError(t, err)

	select {
	case v := <-result:
		assert.Equal(t, "test", v)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive published value")
	}
}

func TestInMemoryTopic_SubscriberOnlyGetsNewEvents(t *testing.T) {
	t.Parallel()
	topic, cleanup := pubsub.NewInMemoryTopic[int](
		pubsub.InMemoryTopicWithPublishBuffer(0),
		pubsub.InMemoryTopicWithMaxBatchWait(0),
	)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Publish before any subscribers
	for i := 1; i <= 3; i++ {
		err := topic.Publish(ctx, i)
		require.NoError(t, err)
	}

	go func() {
		// Ensure subscriber is ready
		time.Sleep(100 * time.Millisecond)
		err := topic.Publish(ctx, 4)
		require.NoError(t, err)
	}()

	for val, err := range topic.Subscribe(ctx) {
		require.NoError(t, err)
		assert.Equal(t, 4, val)
		return
	}

}

func TestInMemoryTopic_MultipleSubscribers(t *testing.T) {
	t.Parallel()
	topic, cleanup := pubsub.NewInMemoryTopic[string]()
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const numSubscribers = 3
	const numMessages = 5

	results := make(chan string, numSubscribers*numMessages)
	var wg sync.WaitGroup
	wg.Add(numSubscribers)

	// Start subscribers
	for i := 0; i < numSubscribers; i++ {
		go func(id int) {
			defer wg.Done()
			count := 0
			for val, err := range topic.Subscribe(ctx) {
				if err != nil {
					return
				}
				results <- fmt.Sprintf("sub%d:%s", id, val)
				count++
				if count >= numMessages {
					return
				}
			}
		}(i)
	}

	// Ensure subscribers are ready
	time.Sleep(20 * time.Millisecond)

	// Publish messages
	for i := 0; i < numMessages; i++ {
		msg := fmt.Sprintf("msg%d", i)
		err := topic.Publish(ctx, msg)
		require.NoError(t, err)
		time.Sleep(5 * time.Millisecond) // Space out publishes
	}

	// Wait for subscribers to process
	wg.Wait()
	close(results)

	// Verify each subscriber received all published messages
	received := make(map[string]int)
	for s := range results {
		received[s]++
	}

	// Each message should be received by each subscriber
	for i := 0; i < numMessages; i++ {
		msg := fmt.Sprintf("msg%d", i)
		for id := 0; id < numSubscribers; id++ {
			key := fmt.Sprintf("sub%d:%s", id, msg)
			assert.Equal(t, 1, received[key], "missing %s", key)
		}
	}
}

func TestInMemoryTopic_CapacityLimitsForSlowSubscriber(t *testing.T) {
	t.Parallel()
	const capacity = 3
	topic, cleanup := pubsub.NewInMemoryTopic[int](
		pubsub.InMemoryTopicOptionCapacity(capacity),
	)
	defer cleanup()
	ctx := context.Background()

	// Slow subscriber
	slowResults := make(chan int, 10)
	go func() {
		for val, err := range topic.Subscribe(ctx) {
			if err != nil {
				return
			}
			slowResults <- val
			time.Sleep(100 * time.Millisecond) // Slow processing
		}
	}()

	// Ensure subscriber is ready
	time.Sleep(10 * time.Millisecond)

	// Publish more messages than capacity
	for i := 1; i <= capacity+2; i++ {
		err := topic.Publish(ctx, i)
		require.NoError(t, err)
		time.Sleep(5 * time.Millisecond)
	}

	// Collect results with timeout
	var received []int
	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case v := <-slowResults:
			received = append(received, v)
		case <-timeout:
			break loop
		}
	}

	// Slow subscriber should have received some messages
	assert.Greater(t, len(received), 0, "slow subscriber got nothing")

	// Check if subscriber got an error on channel
	select {
	case v, ok := <-slowResults:
		if ok {
			received = append(received, v)
		}
	default:
	}
}

func TestInMemoryTopic_ContextCancellation(t *testing.T) {
	t.Parallel()
	topic, cleanup := pubsub.NewInMemoryTopic[string]()
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	subscriberDone := make(chan struct{})

	// Start subscriber
	go func() {
		for val, err := range topic.Subscribe(ctx) {
			if err != nil {
				close(subscriberDone)
				return
			}
			t.Errorf("unexpected value received: %s", val)
		}
	}()

	// Cancel context
	cancel()

	select {
	case <-subscriberDone:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("subscriber did not exit on context cancellation")
	}
}

func BenchmarkPublishNoSubscribers(b *testing.B) {
	topic, cleanup := pubsub.NewInMemoryTopic[int]()
	defer cleanup()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		topic.Publish(ctx, i)
	}
}

func BenchmarkPublishWithSingleSubscriber(b *testing.B) {
	topic, cleanup := pubsub.NewInMemoryTopic[int]()
	defer cleanup()
	ctx := context.Background()

	// Subscriber goroutine
	go func() {
		for range topic.Subscribe(ctx) {
			// simulate consume messages
			time.Sleep(time.Duration(rand.N(1000)) * time.Millisecond)
		}
	}()

	// Wait for subscriber to be ready
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		topic.Publish(ctx, i)
	}

	b.StopTimer()
}

func BenchmarkConcurrentPublishSubscribe(b *testing.B) {
	topic, cleanup := pubsub.NewInMemoryTopic[int]()
	defer cleanup()
	ctx := context.Background()

	const numSubscribers = 10_000

	// Start subscribers that just drain messages
	for i := 0; i < numSubscribers; i++ {
		go func() {
			for range topic.Subscribe(ctx) {
				// simulate consume messages
				time.Sleep(time.Duration(rand.N(1000)) * time.Millisecond)
			}
		}()
	}

	// Wait for subscribers to be ready
	time.Sleep(time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		topic.Publish(ctx, i)
	}
	b.StopTimer()
}
