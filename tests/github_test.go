package sandbox_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode-sandbox/internal/sandbox"

	"github.com/stretchr/testify/require"
)

func TestGitHubCredentialStaysOnHostWhileGuestReceivesAPlaceholder(t *testing.T) {
	const token = "github_pat_FAKE_TEST_CREDENTIAL"
	path := githubTokenPath(t)
	require.NoError(t, os.WriteFile(path, []byte(token+"\n"), 0600))
	s := prepareWithGitHub(t, newWorktree(t), []byte(token))

	require.Equal(t, "$MSB_GH_TOKEN\n", execInSandbox(t, s, "printenv", "GH_TOKEN"))
	stored := storedSandboxConfig(t, s)
	require.Contains(t, stored, token)
	require.NotContains(t, execInSandbox(t, s, "env"), token)
	execInSandbox(t, s, "test", "!", "-e", path)
	require.Equal(t, "$MSB_GH_TOKEN\n", execInSandbox(t, s, "gh", "auth", "token"))
	require.Equal(t, "password=$MSB_GH_TOKEN\n", execInSandbox(t, s, "bash", "-eu", "-c", `
printf 'protocol=https\nhost=github.com\n\n' | git credential fill | grep '^password='`))
}

func TestGitHubInjectionIsRestrictedToAuthorizationOnExactGitHubHosts(t *testing.T) {
	s := prepareWithGitHub(t, newWorktree(t), []byte("github_pat_FAKE"))
	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(storedSandboxConfig(t, s)), &config))
	network, ok := config["network"].(map[string]any)
	require.True(t, ok)
	tls, ok := network["tls"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, tls["enabled"])
	require.Equal(t, true, tls["verify_upstream"])
	secretConfig, ok := network["secrets"].(map[string]any)
	require.True(t, ok)
	secrets, ok := secretConfig["secrets"].([]any)
	require.True(t, ok)
	require.Len(t, secrets, 1)
	secret, ok := secrets[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, secret["require_tls_identity"])
	require.Equal(t, []any{map[string]any{"exact": "api.github.com"}, map[string]any{"exact": "github.com"}}, secret["allowed_hosts"])
	require.Equal(t, map[string]any{"headers": true, "header_fields": []any{"authorization"}, "query": false, "body": false}, secret["substitution"])
}

func TestExistingSandboxKeepsItsOriginalGitHubCredential(t *testing.T) {
	worktree := newWorktree(t)
	s := prepareWithGitHub(t, worktree, []byte("github_pat_FAKE_INITIAL"))
	bootID := execInSandbox(t, s, "cat", "/proc/sys/kernel/random/boot_id")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	resumed, err := sandbox.Prepare(ctx, worktree, nil, githubSecrets(t, []byte("github_pat_FAKE_ROTATED")), testSettings(t))

	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.Equal(t, bootID, execInSandbox(t, resumed, "cat", "/proc/sys/kernel/random/boot_id"))
	stored := storedSandboxConfig(t, resumed)
	require.Contains(t, stored, "github_pat_FAKE_INITIAL")
	require.NotContains(t, stored, "github_pat_FAKE_ROTATED")
}

func TestGitHubCredentialCanBeReusedWithoutChanges(t *testing.T) {
	const token = "github_pat_FAKE_UNCHANGED"
	s := prepareWithGitHub(t, newWorktree(t), []byte(token))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, githubSecrets(t, []byte(token)), testSettings(t))

	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.Equal(t, s.Name, resumed.Name)
}

func TestGitHubCredentialSubstitutionWorksOutsideGuestAndExcludesOtherHeaders(t *testing.T) {
	const token = "github_pat_FAKE_ECHO_TEST_ONLY"
	s := prepareWithGitHub(t, newWorktree(t), []byte(token))

	response := echoedGitHubAuthorization(t, s)

	var echoed struct {
		Headers map[string][]string `json:"headers"`
	}
	require.NoError(t, json.Unmarshal([]byte(response), &echoed))
	require.Equal(t, []string{"Bearer " + token}, echoed.Headers["Authorization"])
	require.Equal(t, []string{"$MSB_GH_TOKEN"}, echoed.Headers["X-Untrusted"])
}

func TestStoppedSandboxKeepsItsOriginalGitHubCredential(t *testing.T) {
	s := prepareWithGitHub(t, newWorktree(t), []byte("github_pat_FAKE_INITIAL"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, s.Stop(ctx))

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, githubSecrets(t, []byte("github_pat_FAKE_ROTATED")), testSettings(t))

	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.Equal(t, "$MSB_GH_TOKEN\n", execInSandbox(t, resumed, "printenv", "GH_TOKEN"))
	require.Contains(t, storedSandboxConfig(t, resumed), "github_pat_FAKE_INITIAL")
	require.NotContains(t, storedSandboxConfig(t, resumed), "github_pat_FAKE_ROTATED")
}

func TestRemovingGitHubTokenFileDoesNotChangeAnExistingSandbox(t *testing.T) {
	path := githubTokenPath(t)
	require.NoError(t, os.WriteFile(path, []byte("github_pat_FAKE_INITIAL"), 0600))
	s := prepareWithGitHub(t, newWorktree(t), []byte("github_pat_FAKE_INITIAL"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	require.NoError(t, os.Remove(path))
	_, err := sandbox.LoadSecrets(githubSecretSettings(path))
	require.Error(t, err)

	resumed, err := sandbox.Prepare(ctx, s.Worktree, nil, nil, testSettings(t))

	require.NoError(t, err)
	defer func() { require.NoError(t, resumed.Close()) }()
	require.Contains(t, storedSandboxConfig(t, resumed), "github_pat_FAKE_INITIAL")
	require.Equal(t, "$MSB_GH_TOKEN\n", execInSandbox(t, resumed, "gh", "auth", "token"))
}

func TestOlderSandboxRequiresRecreationBeforeEnablingGitHub(t *testing.T) {
	s := startSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	_, err := sandbox.Prepare(ctx, s.Worktree, nil, githubSecrets(t, []byte("github_pat_FAKE")), testSettings(t))

	require.ErrorContains(t, err, "recreate this VM")
	require.NotContains(t, storedSandboxConfig(t, s), "github_pat_FAKE")
}

func TestGitHubTokenLoadsOnlyFromDedicatedCredentialFile(t *testing.T) {
	path := githubTokenPath(t)
	require.NoError(t, os.WriteFile(path, []byte("github_pat_FAKE\n"), 0600))
	t.Setenv("GH_TOKEN", "host-gh-token-must-not-be-used")

	secrets, err := sandbox.LoadSecrets(githubSecretSettings(path))

	require.NoError(t, err)
	require.Len(t, secrets, 1)
	require.NoError(t, os.Remove(path))
	_, err = sandbox.LoadSecrets(githubSecretSettings(path))
	require.Error(t, err)
}

func TestGitHubTokenRejectsUnsafePermissionsAndInvalidContents(t *testing.T) {
	for _, tc := range []struct {
		name              string
		dirMode, fileMode os.FileMode
		contents          string
	}{
		{"readable directory", 0755, 0600, "github_pat_FAKE"},
		{"readable token", 0700, 0644, "github_pat_FAKE"},
		{"empty token", 0700, 0600, ""},
		{"multiple lines", 0700, 0600, "first\nsecond"},
		{"NUL byte", 0700, 0600, "first\x00second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := githubTokenPath(t)
			require.NoError(t, os.WriteFile(path, []byte(tc.contents), 0600))
			require.NoError(t, os.Chmod(path, tc.fileMode))
			require.NoError(t, os.Chmod(filepath.Dir(path), tc.dirMode))

			_, err := sandbox.LoadSecrets(githubSecretSettings(path))

			require.Error(t, err)
			if tc.contents != "" {
				require.NotContains(t, err.Error(), tc.contents)
			}
		})
	}
}

func TestGitHubTokenRejectsSymlinks(t *testing.T) {
	path := githubTokenPath(t)
	target := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(target, []byte("github_pat_FAKE"), 0600))
	require.NoError(t, os.Symlink(target, path))

	_, err := sandbox.LoadSecrets(githubSecretSettings(path))

	require.ErrorContains(t, err, "regular owner-only file")
}
