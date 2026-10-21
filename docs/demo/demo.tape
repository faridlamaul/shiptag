# Records docs/assets/demo.gif and docs/assets/dry-run.png.
# Run from the repository root: make demo

Output docs/assets/demo.gif

Require git

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 440
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set WindowBar Colorful
Set BorderRadius 10

Hide
# SOURCE_DATE_EPOCH pins the notes date (2026-10-17) so re-recordings match.
Type `export PATH="$PWD/bin:$PATH" SOURCE_DATE_EPOCH=1792231200 PS1='\[\e[38;5;147m\]rocket\[\e[0m\] $ '` Enter
Type "docs/demo/setup.sh /tmp/shiptag-demo && cd /tmp/shiptag-demo && clear" Enter
Sleep 500ms
Show

Type "git log --oneline v1.2.0..HEAD" Sleep 400ms Enter
Sleep 2s

Type "shiptag next-version" Sleep 400ms Enter
Sleep 2s

Type "clear" Enter
Type "shiptag tag --dry-run | head -10" Sleep 400ms Enter
Sleep 2s
Screenshot docs/assets/dry-run.png
Sleep 3s
