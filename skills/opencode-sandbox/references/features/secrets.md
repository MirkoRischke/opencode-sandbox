# Proxied secrets

Proxied secrets let guest tools authenticate without reading the real
credential. Give the tool the placeholder from its guest environment. The
real value is sent only to the hosts and in the headers you allow.

1. [Use a credential file](#use-a-credential-file)
2. [Use the OS keyring](#use-the-os-keyring)
3. [Limit where a secret can be used](#limit-where-a-secret-can-be-used)
4. [Rotate or revoke a credential](#rotate-or-revoke-a-credential)

## Use a credential file

Add a named secret to your launcher config. This example lets a guest tool
use `$SERVICE_TOKEN` as a bearer token for `api.example.com`:

```toml
[secrets.service]
source.file = "creds/service-token"
guest.env = "SERVICE_TOKEN"
allowed_hosts = ["api.example.com"]
headers = ["authorization"]
```

- Use one source per secret and a unique `guest.env`. A more specific entry
  replaces the whole same-named inherited entry.
- Use a path relative to the config file or an absolute path. Don't use
  symlinks. Set the directory to mode `700` and the file to `600`.
- Store only the credential on one line. Don't commit the file. A missing or
  invalid configured credential stops launch.

For GitHub, follow the [GitHub guide](github.md). The
[gateway key](llm-gateway.md) is automatic, and [AWS roles](aws.md) use
separate settings.

## Use the OS keyring

Replace `source.file` with `source.keyring = {}`. Then store and manage the
entry from the host:

```sh
opencode-sandbox secret set service
opencode-sandbox secret list
opencode-sandbox secret remove service
```

- `set` prompts for hidden input in a terminal. `list` shows names, not
  values. `remove` asks for confirmation. None of these commands edits config.
- The default service is `opencode-sandbox` and the account is the secret
  name. To use custom identifiers, set
  `source.keyring = { service = "my-company", account = "work-service" }`
  and pass matching `--service` and `--account` options to `secret set`.
- A missing or inaccessible entry stops launch. On Ubuntu over SSH or in a
  headless session, check that your user keyring is available and unlocked.

## Limit where a secret can be used

- Set both `allowed_hosts` and `headers`. Use `example.com` for one host
  or `*.example.com` for it and all subdomains. This is different from the
  [network allowlist syntax](networking.md#allow-or-block-internet-access).
- The network policy must allow the destination separately. Substitution uses
  verified TLS by default. Use `allow_http = true` only for a trusted host-local
  service that needs plaintext HTTP.
- Trust every destination you list. It receives the real credential and can
  send it back in a response.

## Rotate or revoke a credential

Existing VMs retain their credentials on the host, even if you change or
remove the source. Recreate the VM to apply a new credential or remove its
copy. To cut off access immediately, revoke the credential with its provider.
