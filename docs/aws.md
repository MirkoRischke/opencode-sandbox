# AWS role credentials

AWS access is opt-in for each launch, not configured in `config.toml`:

```sh
opencode-sandbox --aws-role arn:aws:iam::123456789012:role/SandboxReadOnly

# Optionally select a particular host source profile:
opencode-sandbox --aws-role arn:aws:iam::123456789012:role/SandboxReadOnly \
  --aws-profile my-sso-profile
```

Without `--aws-role`, no host AWS authentication is performed. Any previously
launcher-managed AWS credentials/config in the reused VM are removed. Unmanaged
guest files are left alone. `--aws-profile` is optional, but requires `--aws-role`.

The launcher uses the host AWS SDK credential chain (or the explicitly selected
profile) to call STS `AssumeRole`. Sign in to SSO on the host first if required.
The source identity must be permitted to assume the role, and the role's trust
policy must allow it. No interactive MFA flow is implemented.

Every request uses a duration of **3600 seconds** and the sandbox name as the
session name. Region resolution uses host `AWS_REGION`, then `AWS_DEFAULT_REGION`,
then the selected/default host profile's region, falling back to **`eu-west-1`**.
There are no region or duration flags/config keys.

## Guest files and refresh

Before OpenCode starts, the launcher transfers only temporary role credentials
through the SDK's filesystem operations into guest `/root/.aws/credentials`, using the `[default]`
profile. It writes the resolved region to `/root/.aws/config`. The directory is
mode `700` and both files are mode `600`. No host credentials file or SSO cache is
mounted, and no `AWS_SHARED_CREDENTIALS_FILE` variable is needed.

AWS CLI is installed in the base image. Rebuild/import it with `mise run load`,
then update the launcher with `mise run install`. Existing VMs need recreation to
gain the CLI; AWS credentials themselves are replaced on every launch without
recreation. Existing unmanaged guest AWS files are not overwritten: remove or
relocate them deliberately before enabling this feature.

The host refreshes credentials five minutes before expiry, atomically replacing
the guest file. New AWS CLI invocations pick them up; long-running SDK clients
may cache credentials and are **not supported for refresh**. Failures are reported
without credential contents and retried every 30 seconds. Old credentials expire
naturally; no broader credentials are substituted. An initial failure refuses
launch. The refresh loop is canceled and joined before the VM stops on exit.

The stopped VM retains its last credentials on its disk. On the next launch they
are replaced, or removed if AWS is disabled. Files are marked as launcher-managed
using `/root/.aws/.opencode-sandbox-managed`.

## Permissions and networking

This differs from `[secrets]`: sandbox code **can read and exfiltrate the temporary
AWS credentials**. Use a narrowly scoped role, preferably read-only where possible.
Never grant broad source-identity credentials directly to the guest.

Network allowlisting is independent. Add only the required guest AWS service
hosts under `[network].allowed_hosts`, e.g. `sts.eu-west-1.amazonaws.com` for
`aws sts get-caller-identity`. Other services, regions, S3 bucket endpoints, and
redirects need their own hosts. Host role assumption is outside the guest's
network policy and does not require allowing STS inside the VM.
