on «event emalcpma» theMessages given «class pmar»:theRule
    try
        set handledCount to count theMessages
    on error messageText number numberValue
        return messageText & numberValue
    end try
    return handledCount
end «event emalcpma»

on run argv
    return argv
end run
