package samplearchive_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"main/internal/testutil"
)

// fakeSampleImageGenerator 只验证 Recorder 传给端口的路径与写出的引用，不做图像计算。
type fakeSampleImageGenerator struct {
	candidateCID string
	baselineCIDs []string
	err          error
	gotCandidate string
	gotBaselines []string
}

func (f *fakeSampleImageGenerator) Generate(_ context.Context, candidate string, baselines []string) (string, []string, error) {
	f.gotCandidate = candidate
	f.gotBaselines = append([]string(nil), baselines...)
	return f.candidateCID, f.baselineCIDs, f.err
}

type fakePromptReader struct {
	data []byte
}

func (f *fakePromptReader) Prompt(string) []byte { return f.data }

func newTestRecorder(t *testing.T, generator samplearchive.SampleImageGenerator) (*samplearchive.Recorder, string) {
	t.Helper()
	dataRoot := t.TempDir()
	recorder, err := samplearchive.NewRecorder(dataRoot, dataRoot, generator, &fakePromptReader{data: []byte(`{"prompt":"hello"}`)})
	require.NoError(t, err)
	return recorder, dataRoot
}

func readRecordLines(t *testing.T, dataRoot string) []map[string]any {
	t.Helper()
	lines, err := os.ReadFile(filepath.Join(dataRoot, "samples.jsonl"))
	require.NoError(t, err)
	var records []map[string]any
	for _, line := range splitLines(string(lines)) {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func TestRecorder_Record_AppendsRecordReferencingCandidateAndBaselineCIDs(t *testing.T) {
	generator := &fakeSampleImageGenerator{
		candidateCID: "B-candidate",
		baselineCIDs: []string{"B-baseline"},
	}
	recorder, dataRoot := newTestRecorder(t, generator)

	sample := &session.Sample{
		ImageID:       scalar.ToID("img-1"),
		DirectoryID:   scalar.ToID("dir-1"),
		SessionID:     scalar.ToID("session-1"),
		Round:         2,
		Action:        shared.ImageActionReject,
		Timestamp:     time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC),
		SourceRelPath: testutil.Rel(t, "source.png"),
		Baselines: []session.SampleImage{{
			ID:      scalar.ToID("base-1"),
			RelPath: testutil.Rel(t, "baseline.png"),
		}},
	}
	require.NoError(t, recorder.Record(context.Background(), sample))

	records := readRecordLines(t, dataRoot)
	require.Len(t, records, 1)
	assert.Equal(t, "img-1", records[0]["imageId"])
	assert.Equal(t, "dir-1", records[0]["directoryId"])
	assert.Equal(t, "session-1", records[0]["sessionId"])
	assert.Equal(t, float64(2), records[0]["round"])
	assert.Equal(t, "REJECT", records[0]["action"])
	assert.Equal(t, "B-candidate", records[0]["candidateCid"])
	assert.Equal(t, []any{"B-baseline"}, records[0]["baselineCids"])
	assert.Equal(t, []any{"base-1"}, records[0]["baselineImageIds"])
	// rating 仅作为 action 的 XMP 载体，不进入 v2 归档
	assert.NotContains(t, records[0], "rating")
	assert.NotContains(t, records[0], "thumbnailCid")
	// SHA256("hello")，由 pwsh 预先算出
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", records[0]["fingerprint"])

	assert.Equal(t, filepath.Join(dataRoot, "source.png"), generator.gotCandidate)
	require.Len(t, generator.gotBaselines, 1)
	assert.Equal(t, "baseline.png", filepath.Base(generator.gotBaselines[0]))
}

func TestRecorder_Record_WithoutBaselines_WritesEmptyLists(t *testing.T) {
	generator := &fakeSampleImageGenerator{candidateCID: "B-candidate"}
	recorder, dataRoot := newTestRecorder(t, generator)

	require.NoError(t, recorder.Record(context.Background(), &session.Sample{
		ImageID:       scalar.ToID("img-1"),
		Action:        shared.ImageActionReject,
		SourceRelPath: testutil.Rel(t, "source.png"),
	}))

	records := readRecordLines(t, dataRoot)
	require.Len(t, records, 1)
	assert.Equal(t, []any{}, records[0]["baselineCids"])
	assert.Equal(t, []any{}, records[0]["baselineImageIds"])
	assert.Empty(t, generator.gotBaselines)
}

func TestRecorder_Record_AppendsOneRecordPerSample(t *testing.T) {
	generator := &fakeSampleImageGenerator{candidateCID: "B-candidate", baselineCIDs: []string{"B-base"}}
	recorder, dataRoot := newTestRecorder(t, generator)

	for i := range 2 {
		require.NoError(t, recorder.Record(context.Background(), &session.Sample{
			ImageID:       scalar.ToID("img-1"),
			Action:        shared.ImageActionKeep,
			Round:         i,
			SourceRelPath: testutil.Rel(t, "source.png"),
			Baselines:     []session.SampleImage{{ID: scalar.ToID("base-1"), RelPath: testutil.Rel(t, "baseline.png")}},
		}))
	}

	assert.Len(t, readRecordLines(t, dataRoot), 2)
}

func TestRecorder_Record_GeneratorErrorPropagatesWithoutAppending(t *testing.T) {
	recorder, dataRoot := newTestRecorder(t, &fakeSampleImageGenerator{err: errors.New("magick failed")})

	err := recorder.Record(context.Background(), &session.Sample{ImageID: scalar.ToID("img-1")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "magick failed")
	_, statErr := os.Stat(filepath.Join(dataRoot, "samples.jsonl"))
	assert.True(t, os.IsNotExist(statErr))
}

func splitLines(content string) []string {
	var lines []string
	start := 0
	for i, r := range content {
		if r == '\n' {
			lines = append(lines, content[start:i])
			start = i + 1
		}
	}
	return lines
}
