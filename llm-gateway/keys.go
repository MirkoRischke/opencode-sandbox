package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// EnsureTeam creates one persistent LiteLLM team per repository. Call Ensure first.
func (g *Gateway) EnsureTeam(ctx context.Context, repositoryID, name string) (string, error) {
	var teamID string
	err := g.withCredentialsLock(ctx, func() error {
		path := filepath.Join(g.directory, "credentials", "team-"+repositoryID)
		stored, err := os.ReadFile(path)
		if err == nil {
			teamID = string(stored)
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read gateway team: %w", err)
		}
		teamID = "oc-" + repositoryID
		if err := g.request(ctx, "/team/new", map[string]any{
			"team_id": teamID, "team_alias": name,
		}, nil); err != nil {
			return err
		}
		return writeOnce(path, []byte(teamID))
	})
	return teamID, err
}

// EnsureKey reuses a VM's credential across launches. All proxy models are allowed;
// management routes are not. Credentials never belong in guest files or logs.
func (g *Gateway) EnsureKey(ctx context.Context, teamID, vmName string) (string, error) {
	var key string
	err := g.withCredentialsLock(ctx, func() error {
		path := g.keyPath(vmName)
		stored, err := os.ReadFile(path)
		if err == nil {
			key = string(stored)
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read gateway key: %w", err)
		}
		var response struct {
			Key string `json:"key"`
		}
		if err := g.request(ctx, "/key/generate", map[string]any{
			"team_id": teamID, "key_alias": vmName,
			"models": []string{"all-proxy-models"}, "key_type": "llm_api",
		}, &response); err != nil {
			return err
		}
		if response.Key == "" {
			return fmt.Errorf("gateway returned an empty API key")
		}
		key = response.Key
		return writeOnce(path, []byte(key))
	})
	return key, err
}

// DeleteKey attempts remote deletion once and discards the local credential even
// on failure. Repository teams and their historical usage are retained.
func (g *Gateway) DeleteKey(ctx context.Context, vmName string) error {
	if _, err := os.Stat(g.keyPath(vmName)); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect gateway key: %w", err)
	}
	return g.withCredentialsLock(ctx, func() error {
		path := g.keyPath(vmName)
		key, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read gateway key for deletion: %w", err)
		}
		remoteErr := g.request(ctx, "/key/delete", map[string]any{"keys": []string{string(key)}}, nil)
		if remoteErr != nil {
			remoteErr = fmt.Errorf("delete gateway key for %s (delete it manually in LiteLLM): %w", vmName, remoteErr)
		}
		localErr := os.Remove(path)
		if localErr != nil {
			localErr = fmt.Errorf("remove local gateway key: %w", localErr)
		}
		return errors.Join(remoteErr, localErr)
	})
}

func (g *Gateway) keyPath(vmName string) string {
	return filepath.Join(g.directory, "credentials", "key-"+vmName)
}

func (g *Gateway) withCredentialsLock(ctx context.Context, run func() error) error {
	if err := os.MkdirAll(filepath.Join(g.directory, "credentials"), 0700); err != nil {
		return fmt.Errorf("create gateway credentials directory: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(g.directory, ".credentials.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open gateway credentials lock: %w", err)
	}
	defer lock.Close() //nolint:errcheck // Closing releases the advisory lock.
	if err := acquire(ctx, lock); err != nil {
		return err
	}
	return run()
}

func (g *Gateway) request(ctx context.Context, endpoint string, payload, result any) error {
	env, err := os.ReadFile(filepath.Join(g.directory, ".env"))
	if err != nil {
		return fmt.Errorf("read gateway administrative credential: %w", err)
	}
	var masterKey string
	for _, line := range strings.Split(string(env), "\n") {
		if value, ok := strings.CutPrefix(line, "LITELLM_MASTER_KEY="); ok {
			masterKey = value
		}
	}
	if masterKey == "" {
		return fmt.Errorf("gateway .env is missing LITELLM_MASTER_KEY")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode gateway request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create gateway request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+masterKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("gateway %s: %w", endpoint, err)
	}
	defer response.Body.Close() //nolint:errcheck // Response is consumed or discarded below.
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Error bodies can echo credentials. Report status, never the body.
		return fmt.Errorf("gateway %s: HTTP %d", endpoint, response.StatusCode)
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
			return fmt.Errorf("decode gateway response: %w", err)
		}
	}
	return nil
}
