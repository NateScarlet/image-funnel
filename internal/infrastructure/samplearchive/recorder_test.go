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
)

type fakeThumbnailer struct {
	data []byte
	err  error
}

func (f *fakeThumbnailer) Thumbnail(context.Context, string) ([]byte, error) {
	return f.data, f.err
}

type fakePromptReader struct {
	data []byte
}

func (f *fakePromptReader) Prompt(string) []byte { return f.data }

func TestRecorder_Record_AppendsRecordReferencingCID(t *testing.T) {
	dataRoot := t.TempDir()
	thumbData := []byte("thumbnail-bytes")
	recorder, err := samplearchive.NewRecorder(dataRoot,
		&fakeThumbnailer{data: thumbData},
		&fakePromptReader{data: []byte(`{"prompt":"hello"}`)},
	)
	require.NoError(t, err)

	sample := &session.Sample{
		ImageID:     scalar.ToID("img-1"),
		DirectoryID: scalar.ToID("dir-1"),
		SessionID:   scalar.ToID("session-1"),
		Round:       2,
		Action:      shared.ImageActionReject,
		Rating:      -1,
		Timestamp:   time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC),
		SourcePath:  filepath.Join(dataRoot, "source.png"),
	}
	require.NoError(t, recorder.Record(context.Background(), sample))

	lines, err := os.ReadFile(filepath.Join(dataRoot, "samples.jsonl"))
	require.NoError(t, err)
	var records []map[string]any
	for _, line := range splitLines(string(lines)) {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	require.Len(t, records, 1)
	assert.Equal(t, "img-1", records[0]["imageId"])
	assert.Equal(t, "dir-1", records[0]["directoryId"])
	assert.Equal(t, "session-1", records[0]["sessionId"])
	assert.Equal(t, float64(2), records[0]["round"])
	assert.Equal(t, "REJECT", records[0]["action"])
	assert.Equal(t, float64(-1), records[0]["rating"])
	assert.NotEmpty(t, records[0]["timestamp"])

	cid := "BAFKREIEV6F62YQSGLZEQVXUQGKGGKDOEZP3TJEUNK5RYTGJI2JLT7MZQAI"
	assert.Equal(t, cid, records[0]["thumbnailCid"])
	// SHA256("hello")，由 pwsh 预先算出
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		records[0]["fingerprint"])

	h := cid[1:]
	stored, err := os.ReadFile(filepath.Join(dataRoot, "cas", h[len(h)-3:len(h)-1], h+".data"))
	require.NoError(t, err)
	assert.Equal(t, thumbData, stored)
}

func TestRecorder_Record_DeduplicatesThumbnailButKeepsEveryEvent(t *testing.T) {
	dataRoot := t.TempDir()
	recorder, err := samplearchive.NewRecorder(dataRoot,
		&fakeThumbnailer{data: []byte("thumbnail-bytes")},
		&fakePromptReader{},
	)
	require.NoError(t, err)

	for i := range 2 {
		require.NoError(t, recorder.Record(context.Background(), &session.Sample{
			ImageID: scalar.ToID("img-1"),
			Action:  shared.ImageActionKeep,
			Rating:  5,
			Round:   i,
		}))
	}

	lines, err := os.ReadFile(filepath.Join(dataRoot, "samples.jsonl"))
	require.NoError(t, err)
	assert.Len(t, splitLines(string(lines)), 2)

	h := "AFKREIEV6F62YQSGLZEQVXUQGKGGKDOEZP3TJEUNK5RYTGJI2JLT7MZQAI"
	entries, err := os.ReadDir(filepath.Join(dataRoot, "cas", h[len(h)-3:len(h)-1]))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestRecorder_Record_ThumbnailErrorPropagatesWithoutAppending(t *testing.T) {
	dataRoot := t.TempDir()
	recorder, err := samplearchive.NewRecorder(dataRoot,
		&fakeThumbnailer{err: errors.New("magick failed")},
		&fakePromptReader{},
	)
	require.NoError(t, err)

	err = recorder.Record(context.Background(), &session.Sample{ImageID: scalar.ToID("img-1")})
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
