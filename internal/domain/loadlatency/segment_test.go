package loadlatency

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRunID(t *testing.T) {
	for _, tc := range []struct {
		name    string
		runID   string
		wantErr bool
	}{
		{name: "valid", runID: "0123456789abcdef0123456789abcdef"},
		{name: "empty", runID: "", wantErr: true},
		{name: "too short", runID: "0123456789abcdef0123456789abcde", wantErr: true},
		{name: "too long", runID: "0123456789abcdef0123456789abcdef0", wantErr: true},
		{name: "uppercase", runID: "0123456789ABCDEF0123456789abcdef", wantErr: true},
		{name: "non hex", runID: "0123456789abcdef0123456789abcdeg", wantErr: true},
		{name: "with hyphen", runID: "01234567-89ab-cdef-0123-456789abcdef", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRunID(tc.runID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateSegmentSize(t *testing.T) {
	require.NoError(t, ValidateSegmentSize(nil))
	require.NoError(t, ValidateSegmentSize([]byte(strings.Repeat("a", MaxSegmentBytes))))
	require.Error(t, ValidateSegmentSize([]byte(strings.Repeat("a", MaxSegmentBytes+1))))
}

func TestSegmentPathIsPureFunctionOfRunID(t *testing.T) {
	dir := t.TempDir()

	got := SegmentPath(dir, "0123456789abcdef0123456789abcdef")
	assert.Equal(t, filepath.Join(dir, "0123456789abcdef0123456789abcdef.jsonl"), got)
	assert.True(t, filepath.IsAbs(got), "绝对路径由目录决定，不得退化为工作区相对路径")

	assert.Equal(t, got, SegmentPath(dir, "0123456789abcdef0123456789abcdef"))
}
