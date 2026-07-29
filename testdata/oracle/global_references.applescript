property saved_value : "initial"

on globalProbe()
    global saved_value
    set saved_value to "changed"
    display dialog saved_value
    return result
end globalProbe
