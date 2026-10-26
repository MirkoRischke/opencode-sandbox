package sandbox

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAICredentialSelectionPreservesOnlyTheActiveSubscription(t *testing.T) {
	t.Parallel()
	data := []byte(`[
 {"integrationID":"other","active":true,"value":{"type":"api"}},
 {"integrationID":"openai","active":false,"value":{"type":"oauth"}},
 {"integrationID":"openai","active":true,"value":{"type":"oauth","access":"FAKE_TOKEN","custom":42}}
]`)

	payload, err := selectOpenAICredential(data)

	require.NoError(t, err)
	require.JSONEq(t, `[{"integrationID":"openai","active":true,"value":{"type":"oauth","access":"FAKE_TOKEN","custom":42}}]`, string(payload))
}

func TestOpenAICredentialSelectionRejectsAmbiguousOrInvalidExports(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, export, message string
	}{
		{name: "invalid JSON", export: `FAKE_TOKEN`, message: "invalid host credential export"},
		{name: "invalid entry", export: `["FAKE_TOKEN"]`, message: "invalid credential entry"},
		{name: "missing", export: `[]`, message: "exactly one active"},
		{name: "inactive", export: `[{"integrationID":"openai","active":false}]`, message: "exactly one active"},
		{name: "API key", export: `[{"integrationID":"openai","active":true,"value":{"type":"api","key":"FAKE_TOKEN"}}]`, message: "subscription OAuth"},
		{name: "multiple", export: `[{"integrationID":"openai","active":true,"value":{"type":"oauth"}},{"integrationID":"openai","active":true,"value":{"type":"oauth"}}]`, message: "exactly one active"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload, err := selectOpenAICredential([]byte(tc.export))

			require.ErrorContains(t, err, tc.message)
			require.NotContains(t, err.Error(), "FAKE_TOKEN")
			require.Nil(t, payload)
			switch tc.name {
			case "invalid JSON":
				var cause *json.SyntaxError
				require.ErrorAs(t, err, &cause)
			case "invalid entry":
				var cause *json.UnmarshalTypeError
				require.ErrorAs(t, err, &cause)
			}
		})
	}
}
