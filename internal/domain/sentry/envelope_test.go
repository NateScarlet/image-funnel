package sentry

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEnvelopeExtractsEventItem(t *testing.T) {
	envelope := `{"event_id":"0123456789abcdef0123456789abcdef","sent_at":"2026-11-25T00:00:00Z"}` + "\n" +
		`{"type":"event","content_type":"application/json"}` + "\n" +
		`{"message":"boom","fingerprint":["submit-nav"]}` + "\n"

	parsed, err := ParseEnvelope([]byte(envelope))
	require.NoError(t, err)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", parsed.EventID)
	assert.JSONEq(t, `{"message":"boom","fingerprint":["submit-nav"]}`, string(parsed.EventJSON))
}

func TestParseEnvelopeUsesDeclaredLengthForPayload(t *testing.T) {
	payload := `{"message":"line1\nline2"}`
	envelope := `{"event_id":"abc"}` + "\n" +
		`{"type":"event","length":` + strconv.Itoa(len(payload)) + `}` + "\n" +
		payload + "\n"

	parsed, err := ParseEnvelope([]byte(envelope))
	require.NoError(t, err)
	assert.JSONEq(t, payload, string(parsed.EventJSON))
}

func TestParseEnvelopeRejectsMissingEvent(t *testing.T) {
	envelope := `{"event_id":"abc"}` + "\n" +
		`{"type":"attachment","length":3}` + "\n" +
		`abc` + "\n"

	_, err := ParseEnvelope([]byte(envelope))
	require.Error(t, err)
}
