---
description: Set up relgap's shell completion for bash, zsh, fish, PowerShell, nushell and other shells, and debug it.
---

# Shell completion

A release archive ([Install](/install)) holds `completions/relgap.bash`, `.zsh`, `.fish` and
`.ps1`. With mise or `go install`, generate the script with `relgap completion <shell>`.
[`relgap completion`](/cli/completion) lists every shell it writes a script for.

Completion reads local data only. Tool names for `digest`, `resolve`, `registry test` and
`drift --tool` come from the embedded registries and the `tools:` of your config, never from
`mise registry` or GitHub. A lookup that takes more than 1 s offers nothing.

## bash

bash needs the bash-completion package. Copy the script, or generate it:

```sh
mkdir -p ~/.local/share/bash-completion/completions
relgap completion bash > ~/.local/share/bash-completion/completions/relgap
```

macOS ships bash 3.2 as `/bin/bash`: it needs the `bash-completion` package (version 1), not
`bash-completion@2`, which needs bash 4.2 or later. With bash before 4.4, a comma list such as
`--fail-on breaking,` gets a space after each completed value.

## zsh

Put `_relgap` in a directory on `$fpath` before `compinit` runs:

```sh
mkdir -p ~/.zsh/completions
relgap completion zsh > ~/.zsh/completions/_relgap
# ~/.zshrc, before compinit:
fpath=(~/.zsh/completions $fpath)
autoload -U compinit && compinit
```

If zsh prints
"command not found: compdef", the script loaded before `compinit`. If a new version still completes
the old flags, delete `~/.zcompdump*` and start a new shell. `compaudit` lists group-writable
directories that make `compinit` skip files.

## fish

```sh
relgap completion fish > ~/.config/fish/completions/relgap.fish
```

Don't add `relgap completion fish | source` to `config.fish`: it runs relgap on every shell start.
fish ignores the file extension filter, so `--config` lists every file.

## PowerShell

Save the script and dot-source it from your profile:

```powershell
relgap completion powershell | Out-File -Encoding utf8 ~/relgap.ps1
Add-Content $PROFILE ". ~/relgap.ps1"
```

`$PROFILE` is a
different file in Windows PowerShell 5.1, in PowerShell 7 on Windows and on macOS or Linux, so add
the line to each shell you use. Descriptions show with
`Set-PSReadLineKeyHandler -Key Tab -Function MenuComplete`. PowerShell also ignores the file
extension filter.

## nushell, elvish, xonsh, tcsh, oil and clink

carapace, built into relgap, writes these scripts. Nothing else needs installing. Each script
calls relgap back to get the completions, so relgap must be on `PATH`.

| Shell | Setup |
|---|---|
| nushell | `relgap completion nushell \| save --force ~/.config/nushell/relgap.nu`, then in `config.nu`: `source ~/.config/nushell/relgap.nu` and `$env.config.completions.external = {enable: true, completer: $relgap_completer}` |
| elvish | `eval (relgap completion elvish \| slurp)` in `~/.config/elvish/rc.elv` |
| xonsh | `exec($(relgap completion xonsh))` in `~/.xonshrc` |
| tcsh | ``eval `relgap completion tcsh` `` in `~/.tcshrc` |
| oil | `source <(relgap completion oil)` in your oils startup file |
| clink | `relgap completion clink > %LOCALAPPDATA%\clink\relgap.lua` |

The nushell completer handles every external command, so if you already have one, call
`$relgap_completer` from it when the first span is `relgap`.

## Debugging

`relgap __complete <args> ''` prints what the shell would offer, then `:<directive>`.
`BASH_COMP_DEBUG_FILE=/tmp/comp.log` makes the cobra scripts log each request.
