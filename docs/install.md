# Install and shell completion

## Channels

| Channel | Command | Completion and man pages |
|---|---|---|
| GitHub release | download `relgap_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) | `completions/` and `manpages/` in the archive |
| mise | `mise use github:sassdavid/relgap` | the binary only: generate the script with `relgap completion <shell>` |
| go install | `go install github.com/sassdavid/relgap/cmd/relgap@latest` | the binary only, unsigned |
| Homebrew cask (from M19) | `brew install --cask sassdavid/tap/relgap` | installed for bash, zsh and fish, plus every man page |
| Scoop (from M19) | `scoop bucket add sassdavid https://github.com/sassdavid/scoop-bucket`, then `scoop install relgap` | prints the line for your PowerShell profile |

The archive holds `completions/relgap.bash`, `.zsh`, `.fish` and `.ps1`, and one man page per
command (`manpages/relgap.1.gz`, `manpages/relgap-digest.1.gz` and so on). To read the man pages
from an archive, copy them to a `man1` directory on your `MANPATH`, such as
`~/.local/share/man/man1`.

Completion reads local data only. Tool names for `digest`, `resolve`, `registry test` and
`drift --tool` come from the embedded registries and the `tools:` of your config, never from
`mise registry` or GitHub. A lookup that takes more than 1 s offers nothing.

## bash

bash needs the bash-completion package. Copy the script, or generate it:

```sh
mkdir -p ~/.local/share/bash-completion/completions
relgap completion bash > ~/.local/share/bash-completion/completions/relgap
```

The Homebrew cask installs it to `$(brew --prefix)/etc/bash_completion.d/relgap`. macOS ships
bash 3.2 as `/bin/bash`: it needs brew's `bash-completion` (version 1), not `bash-completion@2`,
which needs bash 4.2 or later. With bash before 4.4, a comma list such as
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

The cask installs it to `$(brew --prefix)/share/zsh/site-functions/_relgap`. If zsh prints
"command not found: compdef", the script loaded before `compinit`. If a new version still completes
the old flags, delete `~/.zcompdump*` and start a new shell. `compaudit` lists group-writable
directories that make `compinit` skip files.

## fish

```sh
relgap completion fish > ~/.config/fish/completions/relgap.fish
```

The cask installs it to `$(brew --prefix)/share/fish/vendor_completions.d/relgap.fish`. Don't
add `relgap completion fish | source` to `config.fish`: it runs relgap on every shell start.
fish ignores the file extension filter, so `--config` lists every file.

## PowerShell

Save the script and dot-source it from your profile:

```powershell
relgap completion powershell | Out-File -Encoding utf8 ~/relgap.ps1
Add-Content $PROFILE ". ~/relgap.ps1"
```

With Scoop, dot-source the script from the `current` directory, which Scoop moves to each new
version. `scoop install` prints the line, such as
`. "$env:USERPROFILE\scoop\apps\relgap\current\completions\relgap.ps1"`. `$PROFILE` is a
different file in Windows PowerShell 5.1, in PowerShell 7 on Windows and on macOS or Linux, so add
the line to each shell you use. Descriptions show with
`Set-PSReadLineKeyHandler -Key Tab -Function MenuComplete`. PowerShell also ignores the file
extension filter.

## nushell, elvish, xonsh, tcsh, oil and clink

carapace, built into relgap, writes these scripts. Nothing else needs installing. Each script
calls `relgap _carapace` back, so relgap must be on `PATH`.

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
