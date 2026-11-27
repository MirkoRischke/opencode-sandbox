# GitHub access in the sandbox

Push feature branches and comment on pull requests as your own GitHub App bot.
Use `gh` and HTTPS Git without putting credentials inside the VM. The supplied
images include `gh`. If you bring a [custom image](vm-images.md), install `gh`
in it first.

1. [Set up a GitHub App](#set-up-a-github-app)
2. [Use GitHub inside the VM](#use-github-inside-the-vm)
3. [Protect the default branch](#protect-the-default-branch)
4. [Configure a personal access token](#configure-a-personal-access-token)

## Set up a GitHub App

1. In the GitHub account that owns your repos, [register a private GitHub App](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/registering-a-github-app).
   Choose a name that identifies your sandbox, for example `Acme Bob Sandbox`,
   and use your account or org page as the homepage URL. Disable the webhook.
   Grant **Contents: Read and write** and **Pull requests: Read and write**.
   The [example manifest](../config/github-app-manifest.json) has the same
   settings for teams that use GitHub’s manifest flow.
2. In the app’s settings, note the **App ID**, generate a private key, and
   [install the app](https://docs.github.com/en/apps/using-github-apps/installing-your-own-github-app)
   on the owner’s account. Choose the repos the sandbox should be able to read.
   **All repositories** includes future repos.
3. Store the downloaded PEM as one line of base64 in your **host** keyring.
   For App ID `123456`, on Wayland:

   ```sh
   base64 -w0 ~/Downloads/github-app.pem | wl-copy
   opencode-sandbox secret set github-app-123456
   ```

   Paste the encoded key into the hidden prompt and press Enter. The entry name
   must match the App ID. The original PEM file is no longer needed by the
   launcher after import.
4. In a parent directory containing your checkouts, for example
   `~/workspace/acme/.opencode-sandbox.toml`, set:

   ```toml
   [github]
   owner = "acme"
   app_id = 123456

   [network]
   allowed_hosts = ["github.com", "api.github.com"]
   ```

   Use your GitHub username as `owner` for personal repos. This config applies
   to workspaces below the directory, including linked worktrees located there.
   Name each original checkout after its GitHub repo; a linked worktree may
   have a different directory name. If these hosts are newly allowed, recreate
   existing VMs to apply the network change.

`[github]` takes precedence over a configured `GH_TOKEN` PAT. If the key is
missing or the app is not installed on the configured owner, launch reports the
error. Workspaces without `[github]` continue to use their existing secret
configuration.

## Use GitHub inside the VM

Launch `opencode-sandbox` as usual. Use HTTPS remotes for Git.

- **Repo covered by the app:** `GH_TOKEN` can write to the workspace repo only.
  Switch to a feature branch, then run `git push -u origin HEAD`. Post a PR
  comment with `gh pr comment 123 --body "Tested in the sandbox"`. GitHub
  attributes the comment to the app’s bot account. To read a different private
  repo covered by the app, use `GH_TOKEN="$GH_READ_TOKEN" gh repo view acme/other-repo`.
- **Plain directory, local-only repo, or repo outside the installation:**
  `GH_TOKEN` is read-only for the repos covered by the app. You can still
  initialize the repo and commit locally. Create the GitHub repo and add it to
  the app’s installation if needed, then relaunch for write access.

The launcher renews one-hour installation tokens while the VM runs. It retries
failed renewals; authenticated operations may fail if the current token expires
before a retry succeeds. Relaunch after changing which repos the app covers to
apply the new access. Snapshot preparation uses read-only access. The generated
[sandbox instructions](sandbox-instructions.md) tell the agent which token to
use in the current session.

## Protect the default branch

GitHub enforces branch protection. For an organization, create an active
[organization branch ruleset](https://docs.github.com/en/organizations/managing-organization-settings/creating-rulesets-for-repositories-in-your-organization).
Target **All repositories** and **Include default branch**. Enable **Restrict
updates** and add the maintainers’ team to the bypass list as **Always allow**.
Leave sandbox apps off the bypass list. If maintainers should be able to push
directly, leave **Require a pull request before merging** off. Use **Restrict
creations** as well if the default branch’s first push must be restricted.
For personal repos, configure a repository ruleset where your plan supports it.

## Configure a personal access token

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

Use `gh` or HTTPS Git with `GH_TOKEN`. Recreate the VM after enabling or
changing the PAT. For immediate revocation, revoke it on GitHub.

See [secret rotation](secrets.md#rotate-or-revoke-a-credential) for why
removing a source doesn't revoke a token in use.
