# Records docs/demo.gif against a simulated toolchain and two fake boards.
#
#   make build
#   sudo demo/sim/setup.sh      # fake /dev/ttyUSB0 + /dev/ttyACM0, sample project
#   vhs demo/demo.tape
#
# Run from the repository root. Linux only (fake serial ports live in /dev).

Output docs/demo.gif


Set Shell "bash"
Set Theme {"name": "esp-workbench", "background": "#0d0c10", "foreground": "#eeeeee", "cursor": "#fab283", "selection": "#282828", "black": "#1e1e1e", "red": "#e06c75", "green": "#7fd88f", "yellow": "#e5c07b", "blue": "#5c9cf5", "magenta": "#9d7cd8", "cyan": "#56b6c2", "white": "#c8c8c8", "brightBlack": "#5a5a5a", "brightRed": "#ef8a91", "brightGreen": "#9fe6aa", "brightYellow": "#f0d29a", "brightBlue": "#82b4f8", "brightMagenta": "#b9a0e6", "brightCyan": "#7ccdd6", "brightWhite": "#ffffff"}
Set FontFamily "JetBrains Mono"
Set FontSize 15
Set LineHeight 1.0
Set Width 1360
Set Height 940
Set Padding 18
Set Margin 28
Set MarginFill "#2a1d14"
Set WindowBar Colorful
Set WindowBarSize 36
Set BorderRadius 12
Set Framerate 30
Set TypingSpeed 70ms

Hide
Type `export PATH="$PWD/bin:$PWD/demo/sim/bin:$PATH" COLORFGBG="15;0" COLORTERM=truecolor && cd /tmp/esp-workbench-demo/blink-rgb && clear`
Enter
Sleep 300ms
Show

Type "esp-workbench"
Sleep 400ms
Enter
# first scan finds two boards; with more than one, you pick
Sleep 3.5s

# pick the ESP32-S3
Enter
Sleep 1.2s

# build
Type "b"
Sleep 3.6s

# build + flash, with live progress
Type "a"
Sleep 7s

# serial monitor streams into the same log, colored by level
Type "m"
Sleep 6.5s
Type "m"
Sleep 1.2s

# partition table read straight off the chip
Type "p"
Sleep 4s
Escape
Sleep 600ms

# flash an existing build, with a confirmation step
Type "x"
Sleep 1.4s
Down
Sleep 700ms
Enter
Sleep 2.4s
Enter
Sleep 3.2s

# erase asks before wiping the chip
Type "e"
Sleep 2.4s
Escape
Sleep 1.6s

# scroll back through the log; it stops following until you press end
Tab
Sleep 500ms
PageUp
Sleep 600ms
PageUp
Sleep 1.8s
Type "G"
Sleep 1s

Type "q"
Sleep 500ms
