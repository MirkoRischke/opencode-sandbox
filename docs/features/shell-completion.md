# Shell completion

Press Tab to discover `opencode-sandbox` commands and flags without looking them
up. Completion is available for Bash, Zsh, Fish, and PowerShell.

For installation instructions, run `opencode-sandbox completion <shell> --help`,
replacing `<shell>` with `bash`, `zsh`, `fish`, or `powershell`.

For Zsh, add this to `~/.zshrc` after `compinit`:

```zsh
source <(opencode-sandbox completion zsh)
```

Zsh normally also completes a simple alias such as `alias ocs=opencode-sandbox`.

For Bash, add this to `~/.bashrc` after loading bash-completion:

```bash
source <(opencode-sandbox completion bash)
```

If you use `alias ocs=opencode-sandbox`, add this after defining the alias and
loading the completion script:

```bash
complete -o default -F __start_opencode-sandbox ocs
```
