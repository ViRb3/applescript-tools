on run
    tell application "System Events"
        set frontApplication to name of process 1 whose frontmost = true
    end tell
    return frontApplication
end run
