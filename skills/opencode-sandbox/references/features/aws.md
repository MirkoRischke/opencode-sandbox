# AWS role credentials

Let a guest tool use AWS without sharing your host's long-lived credentials.
You choose a role, and the guest gets temporary credentials for that role.

1. [Choose an AWS role](#choose-an-aws-role)
2. [Use AWS inside the VM](#use-aws-inside-the-vm)

## Choose an AWS role

Add the role to global `config.toml` or an approved `.opencode-sandbox.toml`:

```toml
[aws]
role = "arn:aws:iam::123456789012:role/SandboxReadOnly"
profile = "my-sso-profile" # optional host source profile
```

For one launch, pass a role and optionally a host profile:

```sh
opencode-sandbox --aws-role arn:aws:iam::123456789012:role/SandboxReadOnly \
  --aws-profile my-sso-profile
```

- Sign in on the host first if the profile uses SSO. The host identity must
  be allowed to assume the role. Interactive MFA is not supported.
- Flags override each setting individually. Use `--aws-role= --aws-profile=`
  to disable configured AWS access for one launch.
- The guest can **read and use the temporary credentials**. Choose a narrowly
  scoped role, preferably read-only. Don't give the guest your host credentials.

## Use AWS inside the VM

Run `aws` in the guest using its default profile. The region comes from
`AWS_REGION`, `AWS_DEFAULT_REGION`, or the host profile, in that order. The
fallback is `eu-west-1`.

- Allow the [service endpoints](networking.md#allow-or-block-internet-access)
  the guest needs. For example, allow `sts.eu-west-1.amazonaws.com` for
  `aws sts get-caller-identity` in `eu-west-1`. Host sign-in doesn't require
  those endpoints to be allowed in the VM.
- New AWS CLI commands pick up refreshed credentials automatically.
  Restart long-running clients if they cache expired credentials.
- Changes apply on the next launch without VM recreation. Existing guest AWS
  config you created yourself is not overwritten. Move it aside before enabling
  the role. When you launch with AWS disabled, sandbox-managed credentials are
  removed.
