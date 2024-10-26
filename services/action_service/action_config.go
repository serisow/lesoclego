package action_service

type ActionConfig struct {
    ID                string                 `json:"id"`
    Label             string                 `json:"label"`
    ActionService     string                 `json:"action_service"`
    Configuration     map[string]interface{} `json:"configuration"`
    ExecutionLocation string                 `json:"execution_location"`
}