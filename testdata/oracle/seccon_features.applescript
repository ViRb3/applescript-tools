use framework "Foundation"
use scripting additions

property nestedValues : {{9, 1}, {7, 3}}
property namedValues : {alpha:9, beta:-7}

on secconFeatureProbe(candidate)
    considering numeric strings
        with timeout of 10 seconds
            set originalValues to {}
            copy originalValues to clonedValues
            set quotientValue to 17 div 5
            set startsCorrectly to candidate starts with "A"
            set endsCorrectly to candidate ends with "Z"
            set generatedCharacter to character id 65
            set nsValue to current application's NSString's stringWithString_(candidate)
            set objectiveCLength to nsValue's |length|()
            return {clonedValues, quotientValue, startsCorrectly, endsCorrectly, generatedCharacter, objectiveCLength, alpha of namedValues}
        end timeout
    end considering
end secconFeatureProbe
