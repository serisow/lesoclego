package action_service

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "strings"
    "github.com/twilio/twilio-go"
    twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
    "github.com/serisow/lesocle/pipeline_type"
)

const SendSMSServiceName = "send_sms"

type SendSMSActionService struct {
    logger *slog.Logger
}

func NewSendSMSActionService(logger *slog.Logger) *SendSMSActionService {
    return &SendSMSActionService{
        logger: logger,
    }
}

func (s *SendSMSActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
        return "", fmt.Errorf("missing action configuration for SendSMSAction")
    }

    // Get credentials from configuration
    config := step.ActionDetails.Configuration
    credentials, err := extractTwilioCredentials(config)
    if err != nil {
        return "", fmt.Errorf("error extracting Twilio credentials: %w", err)
    }

    // Get content from required steps
    requiredSteps := strings.Split(step.RequiredSteps, "\r\n")
    var content string
    
    for _, requiredStep := range requiredSteps {
        requiredStep = strings.TrimSpace(requiredStep)
        if requiredStep == "" {
            continue
        }
        
        stepOutput, ok := pipelineContext.GetStepOutput(requiredStep)
        if !ok {
            return "", fmt.Errorf("required step output '%s' not found for SMS content", requiredStep)
        }
        content += fmt.Sprintf("%v", stepOutput)
    }

    if content == "" {
        s.logger.Error("SMS content is empty",
            slog.String("step_id", step.ID),
            slog.String("required_steps", step.RequiredSteps))
        return "", fmt.Errorf("SMS content is empty")
    }

    // Clean and parse the JSON content
    smsContent := cleanJsonContent(content)
    var smsData struct {
        Message string `json:"message"`
    }
    if err := json.Unmarshal([]byte(smsContent), &smsData); err != nil {
        return "", fmt.Errorf("error parsing SMS content: %w", err)
    }

    if smsData.Message == "" {
        return "", fmt.Errorf("JSON must contain 'message' field")
    }

    // Initialize Twilio client
    client := twilio.NewRestClientWithParams(twilio.ClientParams{
        Username: credentials.AccountSid,
        Password: credentials.AuthToken,
    })

    // Create message params
    params := &twilioApi.CreateMessageParams{
        To:   &credentials.ToNumber,
        From: &credentials.FromNumber,
        Body: &smsData.Message,
    }

    // Send SMS
    message, err := client.Api.CreateMessage(params)
    if err != nil {
        s.logger.Error("Failed to send SMS",
            slog.String("error", err.Error()),
            slog.String("to", credentials.ToNumber))
        return "", fmt.Errorf("failed to send SMS: %w", err)
    }

    // Prepare response
    result := map[string]interface{}{
        "message_sid": *message.Sid,
        "status":     *message.Status,
        "message":    smsData.Message,
    }

    resultJson, err := json.Marshal(result)
    if err != nil {
        return "", fmt.Errorf("error marshaling result: %w", err)
    }

    return string(resultJson), nil
}

func (s *SendSMSActionService) CanHandle(actionService string) bool {
    return actionService == SendSMSServiceName
}

type TwilioCredentials struct {
    AccountSid  string
    AuthToken   string
    FromNumber  string
    ToNumber    string
}

func extractTwilioCredentials(config map[string]interface{}) (*TwilioCredentials, error) {
    credentials := &TwilioCredentials{}
    var ok bool

    if credentials.AccountSid, ok = config["account_sid"].(string); !ok {
        return nil, fmt.Errorf("account_sid not found in config")
    }
    if credentials.AuthToken, ok = config["auth_token"].(string); !ok {
        return nil, fmt.Errorf("auth_token not found in config")
    }
    if credentials.FromNumber, ok = config["from_number"].(string); !ok {
        return nil, fmt.Errorf("from_number not found in config")
    }
    if credentials.ToNumber, ok = config["to_number"].(string); !ok {
        return nil, fmt.Errorf("to_number not found in config")
    }

    return credentials, nil
}