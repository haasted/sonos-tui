# The real client: switch to the third room and open its queue. Run with make
# demo-client.

Output demos/client.gif

Set Shell "bash"
Set FontSize 16
Set Width 900
Set Height 560
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 80ms

Hide
Type make run
Enter
Sleep 4s
Show

Sleep 1s
# Tabs are in speaker id order; 3 jumps to the third.
Type "3"
Sleep 2s

Enter
Sleep 3s
Down
Sleep 600ms
Down
Sleep 600ms
Down
Sleep 1.5s

Escape
Sleep 1s
Hide
Type "q"
Sleep 500ms
