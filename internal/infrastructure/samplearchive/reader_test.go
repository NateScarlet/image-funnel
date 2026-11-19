package samplearchive_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"main/internal/domain/session"
	"main/internal/infrastructure/samplearchive"
	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

func TestReadRecords_ReturnsEveryEvent(t *testing.T) {
	dataRoot := t.TempDir()
	recorder, err := samplearchive.NewRecorder(dataRoot, dataRoot,
		&fakeSampleImageGenerator{candidateCID: "B-candidate", baselineCIDs: []string{"B-base"}},
		&fakePromptReader{},
		zap.NewNop(),
	)
	require.NoError(t, err)
	for i := range 3 {
		require.NoError(t, recorder.Record(context.Background(), &session.Sample{
			ImageID:   scalar.ToID("img-1"),
			Action:    shared.ImageActionKeep,
			Round:     i,
			Baselines: []session.SampleImage{{ID: scalar.ToID("base-1")}},
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

func TestReadRecords_OldFormatFailsFast(t *testing.T) {
	dataRoot := t.TempDir()
	legacy := `{"imageId":"1","sessionId":"s1","action":"KEEP","thumbnailCid":"B-old"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dataRoot, "samples.jsonl"), []byte(legacy), 0o644))

	_, err := samplearchive.ReadRecords(dataRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legacy")
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
	assert.Equal(t, 2, latest[0].Round, "同一图片跨轮次只保留最后决策，其基准集合即该次 Mark 当下的参照")
}

func TestSplit_KeepsSameSessionOnSameSide(t *testing.T) {
	base := time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)
	records := []samplearchive.Record{
		{ImageID: "1", Fingerprint: "fp-a", SessionID: "s1", Timestamp: base},
		{ImageID: "2", Fingerprint: "fp-b", SessionID: "s1", Timestamp: base},
		{ImageID: "3", SessionID: "s2", Timestamp: base},
		{ImageID: "4", SessionID: "s2", Timestamp: base},
	}
	train, test := samplearchive.Split(records, 0.5)

	for _, rec := range test {
		assert.Equal(t, "s1", rec.SessionID, "同一决策链必须同侧，指纹降级为辅助元数据")
	}
	for _, rec := range train {
		assert.Equal(t, "s2", rec.SessionID)
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
			SessionID:   string(rune('s' + rune(i))),
			Timestamp:   base,
		})
	}

	train, test := samplearchive.Split(records, 0.3)
	assert.Len(t, test, 3)
	assert.Len(t, train, 7)
}
