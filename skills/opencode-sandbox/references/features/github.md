# GitHub access in the sandbox

Use `gh` and HTTPS Git to work with private repositories without putting
your GitHub token inside the VM. The supplied images include `gh`. If you
bring a [custom image](vm-images.md), install `gh` in it first.

1. [Configure a token](#configure-a-token)
2. [Use GitHub inside the VM](#use-github-inside-the-vm)

## Configure a token

Save a dedicated **fine-grained, read-only** GitHub personal access token as:

```text
~/.config/opencode-sandbox/creds/github-token
```

Use your configured `XDG_CONFIG_HOME` if set. Follow the
[credential file rules](secrets.md#use-a-credential-file) and add this entry
to `config.toml`. Your host `gh` login is not used.

```toml
[secrets.github]
source.file = "creds/github-token"
guest.env = "GH_TOKEN"
allowed_hosts = ["github.com", "api.github.com"]
headers = ["authorization"]
```

Both hosts must also be allowed by the [network policy](networking.md). To
store the token in the OS keyring instead, use the
[keyring setup](secrets.md#use-the-os-keyring) with the `github` secret name.

Select only the required repositories and grant read-only Contents, Issues, and
Pull requests permissions. GitHub enforces read-only access: the launcher does
not verify or reduce the token's authority. Organization approval may be needed.

## Use GitHub inside the VM

- Run `gh` as usual. It uses the placeholder in `GH_TOKEN`.
- Use HTTPS URLs for Git clone or fetch. An SSH remote won't use this token.
- Recreate the VM after enabling or changing the token. For immediate
  revocation, revoke it on GitHub.

See [secret rotation](secrets.md#rotate-or-revoke-a-credential) for why
removing a source doesn't revoke a token in use.
