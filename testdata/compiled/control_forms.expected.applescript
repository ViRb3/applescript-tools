on exerciseControls(inputValues)
    set total to 0
    repeat 2 times
        set total to total + 1
    end repeat
    repeat while total < 3
        set total to total + 1
    end repeat
    repeat until total ≥ 4
        set total to total + 1
    end repeat
    repeat with counter from 1 to 3 by 1
        set total to total + counter
    end repeat
    repeat with counter from 1 to 5 by 2
        set total to total + counter
    end repeat
    repeat with entry in inputValues
        if entry = missing value then
            exit repeat
        end if
        set total to total + entry
    end repeat
    try
        if total > 0 then
            error "positive" number 17
        end if
    on error messageText number numberValue
        set total to total + numberValue
    end try
    return total
end exerciseControls

on run
    return exerciseControls({1, 2, missing value})
end run
