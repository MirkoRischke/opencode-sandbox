# Keep the OpenCode TUI inside the guest VM

We run both the OpenCode TUI and server inside each worktree's VM, rather than connecting a host TUI to the guest server. The host is a trusted, single-user machine, but guest-controlled code must not gain access to unrelated host files.

A host TUI would naturally reuse host appearance and terminal integrations. However, source inspection of OpenCode 2.0.30 found that it imports server-advertised and locally discovered TUI plugins before applying disable directives such as `plugins: ["-*"]`. Top-level plugin code can therefore execute on the host even when plugin activation is disabled. The guest-writable shared checkout also supplies a local plugin discovery path. Connection authentication does not protect against code supplied by the connected guest.

Keeping the TUI inside the VM contains plugin execution within the same isolation as agent tools. Users can explicitly mount selected host files such as `cli.json` read-only for familiar preferences, without sharing the host configuration directory, session database, or unrelated credentials. No configuration files are shared implicitly. Terminal integration may be less seamless than with a host TUI; that trade-off is intentional.

Reconsider a host TUI only after a pinned client provides a verified, host-controlled way to block all guest/project-supplied plugin code **before discovery/import execution**, including reload and reconnect paths, and other server-directed host execution paths have been reviewed. Alternatively, the client itself must run in a suitable sandbox. Our current evidence is static source inspection, not an executed exploit or an audit of the installed host client.

See [remote TUI plugin isolation research](../research/remote-tui-plugin-isolation.md) for primary-source citations and limitations.
