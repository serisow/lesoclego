package pipeline_type

type Context struct {
    Data map[string]interface{}
    StepOutputs map[string]interface{}
    UserInput   string
    Steps       []PipelineStep  // Added to track all pipeline steps
}

func NewContext() *Context {
    return &Context{
        Data: make(map[string]interface{}),
        StepOutputs: make(map[string]interface{}),
        Steps: make([]PipelineStep, 0),
    }
}

func (c *Context) Set(key string, value interface{}) {
    c.Data[key] = value
}

func (c *Context) Get(key string) (interface{}, bool) {
    val, ok := c.Data[key]
    return val, ok
}

func (c *Context) SetStepOutput(key string, value interface{}) {
    c.StepOutputs[key] = value
}

func (c *Context) GetStepOutput(key string) (interface{}, bool) {
    val, ok := c.StepOutputs[key]
    return val, ok
}

func (c *Context) SetUserInput(input string) {
    c.UserInput = input
}

func (c *Context) GetUserInput() string {
    return c.UserInput
}

// SetSteps sets all the pipeline steps, useful for looking up by output type
func (c *Context) SetSteps(steps []PipelineStep) {
    c.Steps = steps
}

// GetStepByOutputKey finds a pipeline step by its StepOutputKey
func (c *Context) GetStepByOutputKey(outputKey string) (PipelineStep, bool) {
    for _, step := range c.Steps {
        if step.StepOutputKey == outputKey {
            return step, true
        }
    }
    return PipelineStep{}, false
}

// GetStepsByOutputType finds all pipeline steps with a specific OutputType
func (c *Context) GetStepsByOutputType(outputType string) []PipelineStep {
    var matchingSteps []PipelineStep
    for _, step := range c.Steps {
        if step.OutputType == outputType {
            matchingSteps = append(matchingSteps, step)
        }
    }
    return matchingSteps
}