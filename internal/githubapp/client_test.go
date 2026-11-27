package githubapp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestInstallationOnlyTreatsRepositoryNotFoundAsReadOnly(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, repo, accountPath string
		repoStatus              int
		writable                bool
		wantError               string
	}{
		{name: "covered", repo: "widget", accountPath: "/orgs/acme/installation", repoStatus: 200, writable: true},
		{name: "not covered", repo: "widget", accountPath: "/orgs/acme/installation", repoStatus: 404},
		{name: "personal directory", accountPath: "/users/acme/installation"},
		{name: "missing installation", wantError: "not installed on acme"},
		{name: "forbidden", repo: "widget", accountPath: "/orgs/acme/installation", repoStatus: 403, wantError: "HTTP 403"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var paths []string
			client := testClient(t, func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, req.Method)
				paths = append(paths, req.URL.Path)
				if req.URL.Path == test.accountPath {
					return testResponse(200, `{"id":456}`), nil
				}
				if req.URL.Path == "/repos/acme/widget/installation" {
					return testResponse(test.repoStatus, `{"id":456}`), nil
				}
				return testResponse(404, ""), nil
			})
			installation, writable, err := client.Installation(t.Context(), "acme", test.repo)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			require.EqualValues(t, 456, installation)
			require.Equal(t, test.writable, writable)
			if test.repo == "" {
				require.Equal(t, []string{"/orgs/acme/installation", "/users/acme/installation"}, paths)
			}
		})
	}
}

func TestMintScopesReadOnlyAndWriteTokens(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		repo, body string
	}{
		{"", `{"permissions":{"contents":"read","pull_requests":"read"}}`},
		{"widget", `{"repositories":["widget"],"permissions":{"contents":"write","pull_requests":"write"}}`},
	} {
		client := testClient(t, func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "/app/installations/456/access_tokens", req.URL.Path)
			request, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.JSONEq(t, test.body, string(request))
			token, err := json.Marshal(Token{Value: "fixture", ExpiresAt: time.Now().Add(time.Hour)})
			require.NoError(t, err)
			return testResponse(201, string(token)), nil
		})
		token, err := client.Mint(t.Context(), 456, test.repo)
		require.NoError(t, err)
		require.Equal(t, "fixture", token.Value)
	}
}

func testClient(t *testing.T, transport roundTripFunc) *Client {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	client, err := New(123, base64.StdEncoding.EncodeToString(pemBytes))
	require.NoError(t, err)
	client.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "api.github.com", req.URL.Host)
		require.Equal(t, "https", req.URL.Scheme)
		jwt, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
		require.True(t, ok)
		require.Len(t, strings.Split(jwt, "."), 3)
		return transport(req)
	})
	return client
}

func testResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}
