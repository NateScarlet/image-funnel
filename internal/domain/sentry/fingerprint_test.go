package sentry

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryNameJoinsIntoSingleSafeName(t *testing.T) {
	name := DirectoryName([]string{"submit-nav", "pagination-stall", "dir 42"})
	assert.Equal(t, "submit-nav_pagination-stall_dir_42", name)
	assert.NotContains(t, name, "/")
}

func TestDirectoryNameIsFilesystemSafe(t *testing.T) {
	name := DirectoryName([]string{"../evil", "a\\b", "..", ""})
	assert.NotContains(t, name, "..")
	assert.NotContains(t, name, "/")
	assert.NotContains(t, name, "\\")
	assert.NotEqual(t, ".", name)
	assert.NotEqual(t, "..", name)
	assert.NotEmpty(t, name)
}

func TestDirectoryNameDefaultsWhenNoElements(t *testing.T) {
	assert.Equal(t, "default", DirectoryName(nil))
}

func TestDirectoryNameTruncatesWithOmittedCount(t *testing.T) {
	long := strings.Repeat("a", MaxDirectoryNameBytes*2)
	name := DirectoryName([]string{long, "tail"})

	assert.LessOrEqual(t, len(name), MaxDirectoryNameBytes)
	assert.Contains(t, name, "...(")
	assert.Contains(t, name, " more)")
}

func TestEventFingerprintPrefersExplicitValue(t *testing.T) {
	event := []byte(`{"message":"boom","fingerprint":["submit-nav","stalled"]}`)
	assert.Equal(t, []string{"submit-nav", "stalled"}, EventFingerprint(event))
}

func TestEventFingerprintComputedWhenMissing(t *testing.T) {
	first := EventFingerprint([]byte(`{"message":"boom"}`))
	second := EventFingerprint([]byte(`{"message":"other"}`))

	require.Len(t, first, 1)
	assert.NotEmpty(t, first[0])
	assert.NotEqual(t, first, second)
}
