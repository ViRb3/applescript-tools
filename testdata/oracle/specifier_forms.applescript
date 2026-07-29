on exerciseSpecifiers(inputValues)
    set indexedValue to item 2 of inputValues
    set rangedValues to items 2 thru 3 of inputValues
    set firstValue to first item of inputValues
    set lastValue to last item of inputValues
    set middleValue to middle item of inputValues
    set someValue to some item of inputValues
    set allValues to every item of inputValues
    set beginningValue to beginning of inputValues
    set endValue to end of inputValues
    return {indexedValue, rangedValues, firstValue, lastValue, middleValue, someValue, allValues, beginningValue, endValue}
end exerciseSpecifiers

return exerciseSpecifiers({"a", "b", "c", "d"})
