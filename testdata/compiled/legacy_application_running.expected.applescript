on run
    tell application "System Preferences"
        set current pane to pane id "com.apple.preference.general"
    end tell
    return running of application "System Events" = true
end run
