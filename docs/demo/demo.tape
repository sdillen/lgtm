# Demo für die README. Rendern aus dem Repo-Stamm:
#   vhs docs/demo/demo.tape
# Benötigt vhs, ttyd und ffmpeg im PATH sowie die Schrift Iosevka (FontFamily "Iosevka").

Output docs/demo/demo.gif

Set Shell "bash"
Set FontFamily "Iosevka"
Set FontSize 20
Set Width 1600
Set Height 900
Set Padding 20
Set Theme "TokyoNight"
Set TypingSpeed 70ms

# Beispiel-Repo aufbauen und lgtm bauen (nicht im Video sichtbar)
Hide
Type "go build -o /tmp/lgtm-demo/bin/lgtm . && bash docs/demo/setup.sh" Enter
Type "export PATH=/tmp/lgtm-demo/bin:$PATH XDG_CONFIG_HOME=/tmp/lgtm-demo/xdg && cd /tmp/lgtm-demo/repo && clear" Enter
Show

# Start: Überblick über den Branch (erster Tastendruck schließt ihn)
Type "lgtm" Enter
Sleep 3s
Type "j"
Sleep 1s

# Hunks springen und einen Kommentar setzen
Type "]c"
Sleep 1s
Type "c"
Sleep 500ms
Type "add a test for the empty name"
Sleep 1s
Ctrl+S
Sleep 1500ms

# Weiter zur nächsten Änderung
Type "]c"
Sleep 1500ms

# Dateiauswahl
Type " "
Sleep 500ms
Type "p"
Sleep 800ms
Type "main"
Sleep 800ms
Enter
Sleep 2500ms

# Ende
Type ":q!" Enter
Sleep 1s
