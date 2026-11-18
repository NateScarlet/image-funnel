package samplearchive_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"main/internal/domain/session"
	"main/internal/infrastructure/samplearchive"
	"main/internal/scalar"
	"main/internal/shared"
)

func TestReadRecords_ReturnsEveryEvent(t *testing.T) {
	dataRoot := t.TempDir()
	recorder, err := samplearchive.NewRecorder(dataRoot,
		&fakeThumbnailer{data: []byte("thumbnail-bytes")},
		&fakePromptReader{},
	)
	require.NoError(t, err)
	for i := range 3 {
		require.NoError(t, recorder.Record(context.Background(), &session.Sample{
			ImageID: scalar.ToID("img-1"),
			Action:  shared.ImageActionKeep,
			Rating:  5,
			Round:   i,
		}))
	}

	records, err := samplearchive.ReadRecords(dataRoot)
	require.NoError(t, err)
	require.Len(t, records, 3)
	assert.Equal(t, 2, records[2].Round)
}

func TestReadRecords_MissingArchiveReturnsEmpty(t *testing.T) {
	records, err := samplearchive.ReadRecords(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestLatestPerImage_KeepsMaxTimestamp(t *testing.T) {
	base := time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)
	records := []samplearchive.Record{
		{ImageID: "a", Round: 1, Timestamp: base},
		{ImageID: "b", Round: 1, Timestamp: base},
		{ImageID: "a", Round: 2, Timestamp: base.Add(time.Hour)},
	}

	latest := samplearchive.LatestPerImage(records)
	require.Len(t, latest, 2)
	assert.Equal(t, "a", latest[0].ImageID)
	assert.Equal(t, 2, latest[0].Round, "同一图片跨轮次只保留最后决策")
}

func TestSplit_KeepsSameGroupOnSameSide(t *testing.T) {
	base := time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)
	records := []samplearchive.Record{
		{ImageID: "1", Fingerprint: "fp-a", SessionID: "s1", Timestamp: base},
		{ImageID: "2", Fingerprint: "fp-a", SessionID: "s2", Timestamp: base},
		{ImageID: "3", SessionID: "s3", Timestamp: base},
		{ImageID: "4", SessionID: "s3", Timestamp: base},
	}
	// 让 test 拿到指纹组，train 拿到 session 组
	train, test := samplearchive.Split(records, 0.5)

	for _, rec := range test {
		if rec.Fingerprint == "fp-a" {
			for _, other := range test {
				assert.Equal(t, "fp-a", other.Fingerprint, "同指纹必须同侧")
			}
		}
	}
	for _, rec := range train {
		assert.Empty(t, rec.Fingerprint)
		assert.Equal(t, "s3", rec.SessionID, "指纹缺失时按 session id 分组")
	}
	assert.Len(t, train, 2)
	assert.Len(t, test, 2)
}

func TestSplit_RespectsApproximateRatio(t *testing.T) {
	base := time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)
	var records []samplearchive.Record
	for i := range 10 {
		records = append(records, samplearchive.Record{
			ImageID:     string(rune('a' + i)),
			Fingerprint: string(rune('A' + i)),
			Timestamp:   base,
		})
	}

	train, test := samplearchive.Split(records, 0.3)
	assert.Len(t, test, 3)
	assert.Len(t, train, 7)
}
