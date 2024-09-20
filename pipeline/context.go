package pipeline

type Context struct {
    Data map[string]interface{}
    StepOutputs map[string]interface{}
}

func NewContext() *Context {
    return &Context{
        Data: make(map[string]interface{}),
        StepOutputs: make(map[string]interface{}),
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