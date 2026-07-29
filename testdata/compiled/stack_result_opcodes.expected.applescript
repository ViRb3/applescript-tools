on stackResultProbe()
    (log "=== Basic IF ===")
    set x to 10
    if x > 5 then
        (log "x is greater than 5")
    end if
    (log "=== IF…ELSE ===")
    set x to 3
    if x > 5 then
        (log "x is greater than 5")
    else
        (log "x is NOT greater than 5")
    end if
    (log "=== IF…ELSE IF…ELSE chain ===")
    set score to 85
    if score ≥ 90 then
        (log "Grade: A")
    else
        if score ≥ 80 then
            (log "Grade: B")
        else
            if score ≥ 70 then
                (log "Grade: C")
            else
                (log "Grade: F")
            end if
        end if
    end if
    (log "=== One-line IF ===")
    if 2 + 2 = 4 then
        (log "Math works!")
    end if
    (log "=== IF with AND/OR ===")
    set age to 20
    if age ≥ 18 and age ≤ 30 then
        (log "Young adult")
    end if
    if age < 0 or age > 120 then
        (log "Invalid age")
    end if
    (log "=== IF inside a loop ===")
    repeat with n from 1 to 5 by 1
        if n mod 2 = 0 then
            (log n & " is even")
        else
            (log n & " is odd")
        end if
    end repeat
    (log "=== IF checking list contents ===")
    set fruits to {"apple", "orange", "grape"}
    if fruits contains "orange" then
        (log "We have an orange!")
    else
        (log "No orange found.")
    end if
    (log "=== Nested IF blocks ===")
    set x to 10
    set y to 5
    if x > 0 then
        if y > 0 then
            (log "Both numbers are positive")
        end if
    end if
end stackResultProbe

on run
    return stackResultProbe()
end run
