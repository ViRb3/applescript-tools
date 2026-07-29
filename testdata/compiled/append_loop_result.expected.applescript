on appendValues()
    set outputValues to {}
    repeat with currentValue in {1, 2}
        set end of (outputValues) to currentValue
    end repeat
    return outputValues
end appendValues

on run
    appendValues()
end run
