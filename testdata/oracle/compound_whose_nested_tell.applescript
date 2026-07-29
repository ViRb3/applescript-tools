tell application "System Events"
    set matchingProcesses to every process whose not (frontmost is true) and name is not ""
    tell first item of matchingProcesses
        return properties
    end tell
end tell
