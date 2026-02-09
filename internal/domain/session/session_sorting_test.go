package session

import (
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_MarkImage_Sorting(t *testing.T) {
	// Setup images
	img1 := image.NewImage(scalar.ToID("img1"), "img1.jpg", "/path/to/img1.jpg", 1000, time.Now(), nil, 100, 100)
	img2 := image.NewImage(scalar.ToID("img2"), "img2.jpg", "/path/to/img2.jpg", 1000, time.Now(), nil, 100, 100)
	img3 := image.NewImage(scalar.ToID("img3"), "img3.jpg", "/path/to/img3.jpg", 1000, time.Now(), nil, 100, 100)
	images := []*image.Image{img1, img2, img3}

	// Create session
	sess := NewSession(scalar.ToID("sessSort"), scalar.ToID("dir1"), nil, 1, images)
	// NewSession internally creates indices.
	// But in this test, images have IDs "img1", "img2" etc.
	// MarkImage uses ID lookup.

	// Helper to mark with duration
	mark := func(id scalar.ID, durationMs int64) {
		opts := []shared.MarkImageOption{}
		if durationMs > 0 {
			d := scalar.NewDuration(scalar.DurationWithMilliseconds(durationMs))
			opts = append(opts, shared.WithDuration(d))
		}
		err := sess.MarkImage(id, shared.ImageActionKeep, opts...)
		require.NoError(t, err)
	}

	// 1. Mark images with different durations
	// img1: 3000ms
	// img2: 1000ms
	// img3: 2000ms
	mark(scalar.ToID("img1"), 3000)
	mark(scalar.ToID("img2"), 1000)
	mark(scalar.ToID("img3"), 2000)

	assert.Equal(t, 1, sess.currentRound)
	assert.Equal(t, 3, len(sess.queue))

	// Expect order: img2 (1s), img3 (2s), img1 (3s)
	// Last processed was img3. First in sort is img2. No swap.
	assert.Equal(t, scalar.ToID("img2"), sess.images[sess.queue[0]].ID())
	assert.Equal(t, scalar.ToID("img3"), sess.images[sess.queue[1]].ID())
	assert.Equal(t, scalar.ToID("img1"), sess.images[sess.queue[2]].ID())
}

func TestSession_MarkImage_DurationAccumulation(t *testing.T) {
	img1 := image.NewImage(scalar.ToID("img1"), "img1.jpg", "/path/to/img1.jpg", 1000, time.Now(), nil, 100, 100)
	images := []*image.Image{img1}
	sess := NewSession(scalar.ToID("sessAcc"), scalar.ToID("dir1"), nil, 10, images)

	// 1. Mark with 1000ms
	d1 := scalar.NewDuration(scalar.DurationWithMilliseconds(1000))
	err := sess.MarkImage(scalar.ToID("img1"), shared.ImageActionKeep, shared.WithDuration(d1))
	require.NoError(t, err)

	assert.Equal(t, 1000.0, sess.durations[scalar.ToID("img1")].Milliseconds())

	// 2. Undo
	err = sess.Undo()
	require.NoError(t, err)
	// Duration should persist
	assert.Equal(t, 1000.0, sess.durations[scalar.ToID("img1")].Milliseconds(), "Duration should persist after undo")

	// 3. Mark again with 2000ms (Total should be 3000ms)
	d2 := scalar.NewDuration(scalar.DurationWithMilliseconds(2000))
	err = sess.MarkImage(scalar.ToID("img1"), shared.ImageActionKeep, shared.WithDuration(d2))
	require.NoError(t, err)
	assert.Equal(t, 3000.0, sess.durations[scalar.ToID("img1")].Milliseconds())
}

func TestSession_MarkImage_AvoidConsecutiveSameImage(t *testing.T) {
	// Setup images
	img1 := image.NewImage(scalar.ToID("img1"), "img1.jpg", "/path/to/img1.jpg", 1000, time.Now(), nil, 100, 100)
	img2 := image.NewImage(scalar.ToID("img2"), "img2.jpg", "/path/to/img2.jpg", 1000, time.Now(), nil, 100, 100)
	img3 := image.NewImage(scalar.ToID("img3"), "img3.jpg", "/path/to/img3.jpg", 1000, time.Now(), nil, 100, 100)
	// Order queue so img2 is last, allowing us to mark it last without skipping others
	images := []*image.Image{img1, img3, img2}

	// Create session with targetKeep 1
	sess := NewSession(scalar.ToID("sessAvoid"), scalar.ToID("dir1"), nil, 1, images)

	// Helper to mark with duration
	mark := func(id scalar.ID, durationMs int64) {
		d := scalar.NewDuration(scalar.DurationWithMilliseconds(durationMs))
		err := sess.MarkImage(id, shared.ImageActionKeep, shared.WithDuration(d))
		require.NoError(t, err)
	}

	// 1. Mark images in queue order
	// img1: 3000ms
	// img3: 2000ms
	// img2: 1000ms <- Last one processed
	mark(scalar.ToID("img1"), 3000)
	mark(scalar.ToID("img3"), 2000)
	mark(scalar.ToID("img2"), 1000)

	// Normal sort order (by duration): img2 (1s), img3 (2s), img1 (3s)
	// Since img2 was the last of previous round, it should be swapped with img3.
	// Expected order: img3, img2, img1

	require.Equal(t, 1, sess.currentRound)
	require.Equal(t, 3, len(sess.queue))

	assert.Equal(t, scalar.ToID("img3"), sess.images[sess.queue[0]].ID(), "First image should be img3 (swapped)")
	assert.Equal(t, scalar.ToID("img2"), sess.images[sess.queue[1]].ID(), "Second image should be img2 (swapped)")
	assert.Equal(t, scalar.ToID("img1"), sess.images[sess.queue[2]].ID(), "Third image should be img1")
}
