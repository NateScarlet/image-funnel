package sentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseModes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		expected Mode
	}{
		{name: "unset", raw: "", expected: ModeOff},
		{name: "self", raw: SelfDSNValue, expected: ModeSelf},
		{name: "external", raw: "https://abc123@o12345.ingest.sentry.io/67890", expected: ModeExternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode, err := Parse(tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, mode)
		})
	}
}

func TestParseRejectsInvalidValues(t *testing.T) {
	// 自托管标识不接受带后缀的变体，非法 DSN 也必须快速失败而不是静默降级
	for _, raw := range []string{
		"internal.self:extra",
		"internal.self",
		"not-a-dsn",
		"https://o12345.ingest.sentry.io/67890",
		"ftp://key@example.com/1",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := Parse(raw)
			require.Error(t, err)
		})
	}
}

func TestSelfDSNTargetsEnvelopeEndpoint(t *testing.T) {
	dsn, err := SelfDSN("http://localhost:34898")
	require.NoError(t, err)
	assert.Equal(t, "http://self@localhost:34898/0", dsn)
	assert.Equal(t, "/api/0/envelope/", EnvelopePath)
}

func TestSelfDSNRejectsInvalidBaseURL(t *testing.T) {
	for _, baseURL := range []string{"", "not-a-url", "file:///tmp"} {
		t.Run(baseURL, func(t *testing.T) {
			_, err := SelfDSN(baseURL)
			require.Error(t, err)
		})
	}
}

func TestEnvelopeURL(t *testing.T) {
	selfDSN, err := SelfDSN("http://localhost:34898")
	require.NoError(t, err)

	for name, tc := range map[string]struct {
		dsn      string
		endpoint string
		key      string
		wantErr  bool
	}{
		"self": {
			dsn:      selfDSN,
			endpoint: "http://localhost:34898/api/0/envelope/",
			key:      SelfPublicKey,
		},
		"external": {
			dsn:      "https://abc123@o12345.ingest.sentry.io/67890",
			endpoint: "https://o12345.ingest.sentry.io/api/67890/envelope/",
			key:      "abc123",
		},
		"empty":     {dsn: "", wantErr: true},
		"malformed": {dsn: "not-a-dsn", wantErr: true},
		"no key":    {dsn: "https://o12345.ingest.sentry.io/67890", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			endpoint, key, err := EnvelopeURL(tc.dsn)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.endpoint, endpoint)
			assert.Equal(t, tc.key, key)
		})
	}
}
