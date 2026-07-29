on run
    tell application "System Events"
        set matchingProcesses to every process of it whose not (frontmost = true) and name ≠ ""
        tell item 1 of matchingProcesses
            return properties
        end tell
    end tell
end run
