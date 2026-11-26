package sentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const testSalt = "00112233445566778899aabbccddeeff"

func TestScrubStringRemovesPathsAndCredentials(t *testing.T) {
	for name, tc := range map[string]struct {
		input     string
		wantStart string
		secret    string
	}{
		"windows path":  {`open C:\Users\alice\photo.jpg failed`, "open path:", `C:\Users\alice\photo.jpg`},
		"unix path":     {`open /home/alice/.env failed`, "open path:", "/home/alice/.env"},
		"credential":    {`token=s3cr3t-ABCDEFGHIJ`, "token=cred:", "s3cr3t-ABCDEFGHIJ"},
		"bearer header": {`Authorization: Bearer abc123XYZTOKEN`, "Authorization", "abc123XYZTOKEN"},
		"dir id":        {`dir:subdir/风景`, "dir:", "subdir"},
	} {
		t.Run(name, func(t *testing.T) {
			result := scrubString(tc.input, testSalt)
			assert.True(t, len(result) >= len(tc.wantStart), "保留可读前缀：%s", result)
			assert.Contains(t, result, tc.wantStart)
			assert.NotContains(t, result, tc.secret, "原始敏感片段不得保留")
		})
	}
}

func TestScrubStringUsesStableSaltWithinEvent(t *testing.T) {
	first := scrubString(`dir:subdir/风景`, testSalt)
	second := scrubString(`dir:subdir/风景`, testSalt)
	third := scrubString(`dir:subdir/风景`, "ffeeddccbbaa99887766554433221100")
	assert.Equal(t, first, second)
	assert.NotEqual(t, first, third)
}

func TestScrubValueRecursesIntoArraysAndMaps(t *testing.T) {
	input := []any{
		`C:\Users\alice\photo.jpg`,
		map[string]any{"message": `token=abcdefghijklmnop`, "scrubSalt": testSalt},
	}
	result := scrubValue(input, testSalt).([]any)

	assert.NotContains(t, result[0], `C:\Users\alice`)
	entry := result[1].(map[string]any)
	assert.NotContains(t, entry["message"], "abcdefghijklmnop")
	assert.Equal(t, testSalt, entry["scrubSalt"])
}
