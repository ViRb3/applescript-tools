on booleanArguments()
    display dialog "secret" with hidden answer
    display dialog "visible" without hidden answer
    do shell script "true" with administrator privileges
    do shell script "true" without administrator privileges
end booleanArguments
