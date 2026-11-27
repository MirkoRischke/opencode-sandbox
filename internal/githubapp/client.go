// Package githubapp authenticates to GitHub as an app and mints scoped installation tokens.
package githubapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	id   int64
	key  *rsa.PrivateKey
	http *http.Client
}

func New(appID int64, encodedPEM string) (*Client, error) {
	data, err := base64.StdEncoding.DecodeString(encodedPEM)
	if err != nil {
		return nil, fmt.Errorf("decode base64 GitHub App private key: %w", err)
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "RSA PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf("expected the RSA private key PEM downloaded from GitHub")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub App private key: %w", err)
	}
	return &Client{
		id:  appID,
		key: key,
		http: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// Installation finds the app installation on the configured account, then checks
// whether it covers the specified repository. An empty repo skips the repo check.
func (c *Client) Installation(ctx context.Context, owner, repo string) (id int64, writable bool, err error) {
	owner = url.PathEscape(owner)
	id, err = c.installation(ctx, "/orgs/"+owner+"/installation")
	if err == nil && id == 0 {
		id, err = c.installation(ctx, "/users/"+owner+"/installation")
	}
	if err != nil {
		return 0, false, err
	}
	if id == 0 {
		return 0, false, fmt.Errorf("GitHub App %d is not installed on %s", c.id, owner)
	}
	if repo == "" {
		return id, false, nil
	}
	repoID, err := c.installation(ctx, "/repos/"+owner+"/"+url.PathEscape(repo)+"/installation")
	if err != nil {
		return 0, false, err
	}
	if repoID != 0 && repoID != id {
		return 0, false, fmt.Errorf("GitHub repository %s/%s belongs to a different app installation", owner, repo)
	}
	return id, repoID != 0, nil
}

func (c *Client) installation(ctx context.Context, path string) (int64, error) {
	var response struct {
		ID int64 `json:"id"`
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &response); err != nil {
		var status *httpStatusError
		if errors.As(err, &status) && status.code == http.StatusNotFound {
			return 0, nil
		}
		return 0, err
	}
	if response.ID <= 0 {
		return 0, fmt.Errorf("GitHub %s returned no installation ID", path)
	}
	return response.ID, nil
}

type Token struct {
	Value     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Mint requests read access to the installation, or read/write access only to
// repo when a repository name is supplied.
func (c *Client) Mint(ctx context.Context, installationID int64, repo string) (Token, error) {
	request := struct {
		Repositories []string          `json:"repositories,omitempty"`
		Permissions  map[string]string `json:"permissions"`
	}{Permissions: map[string]string{"contents": "read", "pull_requests": "read"}}
	if repo != "" {
		request.Repositories = []string{repo}
		request.Permissions["contents"] = "write"
		request.Permissions["pull_requests"] = "write"
	}
	path := fmt.Sprintf("/app/installations/%d/access_tokens", installationID)
	var token Token
	if err := c.request(ctx, http.MethodPost, path, request, &token); err != nil {
		return Token{}, err
	}
	if token.Value == "" || !token.ExpiresAt.After(time.Now()) {
		return Token{}, fmt.Errorf("GitHub returned an empty or expired installation token")
	}
	return token, nil
}

type httpStatusError struct {
	path string
	code int
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("GitHub %s returned HTTP %d", e.path, e.code)
}

func (c *Client) request(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	jwt, err := c.jwt(time.Now())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request GitHub installation API: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // The read-only HTTP body has no pending writes.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &httpStatusError{path: path, code: resp.StatusCode}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(output); err != nil {
		return fmt.Errorf("decode GitHub response for %s: %w", path, err)
	}
	return nil
}

func (c *Client) jwt(now time.Time) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := fmt.Sprintf(`{"iat":%d,"exp":%d,"iss":%q}`, now.Add(-time.Minute).Unix(),
		now.Add(9*time.Minute).Unix(), strconv.FormatInt(c.id, 10))
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString([]byte(claims))
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(nil, c.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign GitHub App JWT: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
