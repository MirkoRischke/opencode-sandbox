# Sandbox networking

Workspace VMs can reach common development services by default. You control
which internet destinations they can reach and which ports connect the host
and guest.

1. [Allow or block internet access](#allow-or-block-internet-access)
2. [Allow unrestricted public internet](#unrestricted-public-internet)
3. [Open a guest service on your host](#host-to-vm)
4. [Reach a host service from the guest](#vm-to-host)
5. [Apply and troubleshoot changes](#apply-and-troubleshoot-changes)

## Allow or block internet access

Run `opencode-sandbox config default` to see the allowed hosts. GitHub and
common documentation services are included. Package and container registries
are not. Add the hosts your project needs in `.opencode-sandbox.toml`:

```toml
[network]
additional_allowed_hosts = ["registry.npmjs.org", "nodejs.org"]
```

- `allowed_hosts` and `additional_allowed_hosts` work together. Keep your
  shared list in global `allowed_hosts`. A workspace replaces each list it
  sets rather than appending to it.
- Use `example.com` for one host or `.example.com` to include all subdomains.
  Don't use URLs, IP addresses, ports, or `*`.
- Allowed hosts are reachable over HTTPS on port 443. To block application
  internet access, set both lists to `[]` and leave `unrestricted = false`.
- An allowlist limits destinations, not what the guest can send to them. It
  does not widen [secret scope](secrets.md#limit-where-a-secret-can-be-used).

[Snapshot preparation](snapshots.md) has public internet access and can warm
caches for tools you don't want to allow at runtime.

## Unrestricted public internet

To allow public destinations on any port or protocol supported by Microsandbox,
set this globally or in an approved workspace config:

```toml
[network]
unrestricted = true
```

This ignores both host lists. It does not open private networks, extra host
ports, or inbound connections. Secret scopes still apply. Prefer hostnames:
[HTTPS to a raw IP can fail](../limitations/microsandbox.md#msb-08--tls-inspection-can-prevent-direct-ip-https).

## Host to VM

Expose a guest development server to your host browser with an explicit TCP
mapping:

```toml
[[network.host_to_vm]]
host_port = 3000
vm_port = 8080
```

Have the application listen on `0.0.0.0:8080` in the VM, then open
`http://127.0.0.1:3000` on the host. For a container, also publish its port to the
VM. HTTP, HTTPS, WebSockets, and hot reload work over the forwarding; HTTPS still
requires trusting the application's certificate.

- Nothing is exposed by default. Mappings accept TCP only and work from the
  host, not the LAN.
- Pick an unused host port. It is released when the VM stops.
- To clear inherited mappings, set `host_to_vm = []` under `[network]`.

## VM to host

Allow selected host-local TCP services without remapping their ports:

```toml
[network.vm_to_host]
ports = [4000, 5432]
```

Connect from the guest to `host.microsandbox.internal:5432`, not `localhost`.

- The default list allows TCP port `4000` for the [host LLM gateway](llm-gateway.md).
  Keep that port in your list if you use the gateway.
- `ports = []` blocks host services, even with unrestricted public internet.
  The same VM-to-host permissions apply during snapshot preparation.

## Apply and troubleshoot changes

Recreate an existing VM after changing its network policy or port mappings:
run `opencode-sandbox remove`, then launch again. A snapshot restore applies
the current network settings. To inspect blocked destinations, enable
[network diagnostics](diagnostics.md#network-denials).
