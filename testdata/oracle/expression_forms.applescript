on exerciseExpressions(leftValue, rightValue, textValue, flagValue)
    return {leftValue + rightValue, leftValue - rightValue, leftValue * rightValue, leftValue / rightValue, leftValue div rightValue, leftValue mod rightValue, leftValue ^ rightValue, textValue & "!", leftValue is rightValue, leftValue is not rightValue, leftValue < rightValue, leftValue ≤ rightValue, leftValue > rightValue, leftValue ≥ rightValue, textValue contains "needle", textValue starts with "A", textValue ends with "Z", leftValue as text, -leftValue, not flagValue, flagValue and (leftValue < rightValue), flagValue or (leftValue > rightValue)}
end exerciseExpressions

return exerciseExpressions(8, 3, "AneedleZ", false)
