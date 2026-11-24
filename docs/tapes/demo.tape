# gh merge: dry run, then merge across all repos.
Output docs/demo.gif
Set Shell zsh
Set FontSize 16
Set Width 1280
Set Height 720
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Hide
Type "source docs/tapes/env.sh" Enter
Sleep 6s
Show
Type "gh merge --dry-run" Sleep 400ms Enter
Sleep 3s
Type "gh merge --all --rebase-dependabot" Sleep 400ms Enter
Sleep 2.5s
Type "y" Sleep 500ms Enter
Sleep 7s
