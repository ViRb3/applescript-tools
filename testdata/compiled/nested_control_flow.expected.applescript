on nestedControlProbe(x, y)
    if x > 0 then
        try
            if y > 0 then
                return "both"
            else
                return "x only"
            end if
        end try
    else
        return "neither"
    end if
end nestedControlProbe
