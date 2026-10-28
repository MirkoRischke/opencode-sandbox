# GitHub access in the sandbox

Install `gh` by rebuilding/importing the image (`mise run load`), then reinstall
the launcher (`mise run install`). Existing VMs created without GitHub support
must be removed and recreated; secret configuration applies only at VM creation.
Removing a VM deletes its guest-local state, not its
mounted host worktree.

## Host credential

Save a dedicated **fine-grained, read-only** GitHub personal access token as:

```text
~/.config/opencode-sandbox/creds/github-token
```

`XDG_CONFIG_HOME` is respected. The file contains only the token, with an optional
trailing newline. Use directory mode `700` and file mode `600`; the launcher
rejects group/other access and symlinked credential directories or files. Add this
explicit entry to `config.toml`; the file is never read without it:

```toml
[secrets.github]
file = "creds/github-token"
env = "GH_TOKEN"
allowed_hosts = ["github.com", "api.github.com"]
headers = ["authorization"]
```

Both hosts must also be allowed by `[network].allowed_hosts`. A configured file
that is missing or invalid fails launch. Host `gh` credentials are never used.

Select only the required repositories and grant read-only Contents, Issues, and
Pull requests permissions. GitHub enforces read-only access: the launcher does
not verify or reduce the token's authority. Organization approval may be needed.

## Guest behavior

- `gh` uses a placeholder in `GH_TOKEN`, not the real token.
- With the configuration above, preparation restricts replacement to Authorization
  headers sent to exactly `api.github.com` or `github.com`, over verified intercepted TLS.
  Workspace VMs restore a snapshot; Microsandbox 0.7.8 cannot retain selected-header
  restrictions on restore, so replacement can occur in any header on those same hosts.
  See [snapshot limitations](config.md#workspace-snapshots).
- The guest GitHub CLI credential helper authenticates **HTTPS** clone/fetch.
  SSH remotes are not converted or supported by this token mechanism.
- No host credential directory, keychain, or SSH agent is mounted.

Microsandbox **persists the real token in host-side VM configuration**. This is
an accepted tradeoff for the trusted, single-user host; the value is not put in
guest configuration or files. An allowed upstream receives the real credential;
substitution does not guarantee an upstream can never reflect it back.

Credentials are fixed for the lifetime of a VM. Changing or deleting the host
file does not change existing VMs, even after stopping and resuming them. A missing
file still fails launch while its entry remains configured. Removing the entry
allows launch but does not revoke a credential already stored in an existing VM.
Recreate a VM to use a different token or remove its stored credential. To
revoke access immediately, revoke the token on GitHub.
