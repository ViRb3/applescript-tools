property saved_value : "initial"

on globalProbe()
    set saved_value to "changed"
    (display dialog saved_value)
    return result
end globalProbe
