package sentry

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryNameJoinsIntoSingleSafeName(t *testing.T) {
	name := DirectoryName([]string{"submit-nav", "pagination-stall", "dir 42"})
	assert.Equal(t, "submit-nav_pagination-stall_dir 42", name)
	assert.NotContains(t, name, "/")
}

func TestDirectoryNamePreservesUnicode(t *testing.T) {
	assert.Equal(t, "加载超时", DirectoryName([]string{"加载超时"}))
}

func TestSanitizeReplacesOnlyIllegalCharacters(t *testing.T) {
	name := Sanitize(`../evil\id:error?`)

	assert.NotContains(t, name, "..")
	assert.NotContains(t, name, "/")
	assert.NotContains(t, name, `\`)
	assert.NotContains(t, name, ":")
	assert.Contains(t, name, "evil")
	assert.Contains(t, name, "id")
}

func TestSanitizeKeepsWindowsReservedNameSafe(t *testing.T) {
	assert.Equal(t, "\ufffd", Sanitize("CON"))
	assert.Equal(t, "file\ufffd", Sanitize("file. "))
}

func TestDirectoryNameTruncatesUnicodeAtRuneBoundary(t *testing.T) {
	name := DirectoryName([]string{strings.Repeat("超", MaxDirectoryNameBytes)})

	assert.LessOrEqual(t, len(name), MaxDirectoryNameBytes)
	assert.True(t, utf8.ValidString(name), "截断不能把多字节字符切成一半")
	assert.Contains(t, name, "...(")
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

func TestEventFingerprintUsesMessageWithLevel(t *testing.T) {
	event := []byte(`{"level":"error","message":"加载超时"}`)

	assert.Equal(t, []string{"error: 加载超时"}, EventFingerprint(event))

	name := DirectoryName(EventFingerprint(event))
	assert.Contains(t, name, "error")
	assert.Contains(t, name, "加载超时")
	assert.NotContains(t, name, "sha256-")
}

func TestEventFingerprintUsesExceptionTypeAndValue(t *testing.T) {
	event := []byte(`{"exception":{"values":[{"type":"TypeError","value":"boom"}]}}`)
	assert.Equal(t, []string{"TypeError: boom"}, EventFingerprint(event))
}

func TestEventFingerprintUsesExceptionTypeWhenValueEmpty(t *testing.T) {
	event := []byte(`{"exception":{"values":[{"type":"TypeError","value":""}]}}`)
	assert.Equal(t, []string{"TypeError"}, EventFingerprint(event))
}

func TestEventFingerprintUsesLogEntry(t *testing.T) {
	event := []byte(`{"level":"warn","logger":"image-scanner","logentry":"slow load"}`)
	assert.Equal(t, []string{"warn: image-scanner: slow load"}, EventFingerprint(event))
}

func TestEventFingerprintFallsBackToEventTimestamp(t *testing.T) {
	numeric := []byte(`{"timestamp":1764163912.123456}`)
	assert.Equal(t, []string{"Event1764163912123456"}, EventFingerprint(numeric))

	formatted := []byte(`{"timestamp":"2025-11-26T13:31:52.123456Z"}`)
	assert.Equal(t, []string{"Event1764163912123456"}, EventFingerprint(formatted))
}

func TestEventFingerprintFallbackVariesWithEventTime(t *testing.T) {
	first := EventFingerprint([]byte(`{"timestamp":1764163912.123456}`))
	second := EventFingerprint([]byte(`{"timestamp":1764163999.654321}`))

	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.NotEqual(t, first, second)
}

func TestEventFingerprintTimestampKeepsMicroseconds(t *testing.T) {
	first := EventFingerprint([]byte(`{"timestamp":1764163912.000001}`))
	second := EventFingerprint([]byte(`{"timestamp":1764163912.000002}`))

	assert.Equal(t, []string{"Event1764163912000001"}, first)
	assert.Equal(t, []string{"Event1764163912000002"}, second)
}

func TestEventFingerprintPrefersMessageOverExceptionAndLogEntry(t *testing.T) {
	event := []byte(`{"level":"error","message":"boom","exception":{"values":[{"type":"TypeError","value":"bad"}]},"logger":"scanner","logentry":"slow"}`)
	assert.Equal(t, []string{"error: boom"}, EventFingerprint(event))
}
