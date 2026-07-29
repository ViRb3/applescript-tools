on makeWorkflow()
    script Workflow
        property _results : {}

        on newItem()
            script |item|
                property |record| : {|path|:"/tmp"}

                on addItem()
                    set end of ((every item of _results)) to |record|
                end addItem
            end script
            return |item|
        end newItem
    end script
    return Workflow
end makeWorkflow
