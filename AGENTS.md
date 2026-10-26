# Documentation

Update `README.md` only when explicitly asked or to correct information made
outdated by your changes. Exceptions:

- a new command was added

# Implementation

- Handle each error once: either return it (optionally wrapped with context), or log it where handled. Never log and propagate the same error.

- Do not consider migration scenarios; recreation of sandboxes/VMs is acceptable.

# CLI conventions

- Use `remove` (alias `rm`) for removal commands and `list` (alias `ls`) for listing commands, including subcommands.
