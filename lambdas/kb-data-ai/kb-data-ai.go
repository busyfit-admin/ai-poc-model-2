package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
	bedrockagenttypes "github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

func createPrompt(JiraData string) string {
	var ScrumMasterPrompt = fmt.Sprintf(`
	You are an AI Scrum Master. Your task is to estimate points and provide analysis based on the history of Jiras assigned to a specific person. Given the data below for the user, provide a detailed summary of the user's skills and actions taken by them.
	
	User Jira Data in JSON is :
	%s
	
	## End of Jira Data. 

	Instructions:
	
	1. Skills Assessment:
	   - Analyze the Jiras assigned to the user.
	   - Identify the technical and non-technical skills demonstrated.
	   - Highlight any specific expertise or repeated patterns of proficiency.
	
	2. Summary of Actions:
	   - Provide a summary of the actions taken by the user to complete their Jiras.
	   - Mention any notable achievements or contributions.
	   - Identify any trends in the user's performance, such as speed of completion or complexity of tasks handled.

	3. List the Jira Ticket Numbers with Summary Below: 
	   - Keys of Jira Data and Summary
		
	
	Output Format:
	
	1. Skills:
	   - [Skill 1]
	   - [Skill 2]
	   - ...
	
	2. Summary of Actions:
	   - [Action 1]
	   - [Action 2]
	   - ...
	3. List of Jira Tickets: 
		- [Jira Key 1- Summary ] 
		- [Jira Key 2- Summary ]   
	
	Provide the output in string format only and end with the person's name or username`, JiraData)

	return ScrumMasterPrompt
}

var BASE_MODEL_ID = "amazon.titan-text-express-v1"

type SecretManagerClient interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}
type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type BedrockClient interface {
	InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}

type BedrockAgentClient interface {
	RetrieveAndGenerate(ctx context.Context, params *bedrockagentruntime.RetrieveAndGenerateInput, optFns ...func(*bedrockagentruntime.Options)) (*bedrockagentruntime.RetrieveAndGenerateOutput, error)
}

type JiraService struct {
	ctx                context.Context
	logger             *log.Logger
	httpClient         HttpClient
	secretMgrClient    SecretManagerClient
	bedrockClient      BedrockClient
	bedrockAgentClient BedrockAgentClient

	KB_ID string
}

func main() {

	ctx := context.TODO()
	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("ap-south-1"))
	if err != nil {
		fmt.Println("Unable to load SDK config, " + err.Error())
		return
	}

	cookiejar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("Got error while creating cookie jar %s", err.Error())
	}

	// Creating a Transport for control TLS configuration, keep-alives & others
	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}

	httpClient := http.Client{Jar: cookiejar, Transport: tr}

	bedrockClient := bedrockruntime.NewFromConfig(cfg)
	bedrockAgentClient := bedrockagentruntime.NewFromConfig(cfg)

	svc := &JiraService{
		ctx:                ctx,
		logger:             log.New(os.Stdout, "", log.LstdFlags),
		secretMgrClient:    secretsmanager.NewFromConfig(cfg),
		httpClient:         &httpClient,
		bedrockClient:      bedrockClient,
		bedrockAgentClient: bedrockAgentClient,
	}

	svc.KB_ID = os.Getenv("KB_ID")

	lambda.Start(svc.handler)

}

type UserQuery struct {
	Query string `json:"query"`
}

func (svc *JiraService) handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	svc.ctx = ctx

	var apiInput UserQuery
	err := json.Unmarshal([]byte(request.Body), &apiInput)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
		}, err
	}

	kbData, err := svc.GetKBData(apiInput.Query)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
		}, err
	}

	// Create Prompt with the Jira data
	promptData := createPrompt(string(kbData))

	// Invoke Model
	respBody, err := svc.InvokeTitanText(promptData)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
		}, err
	}

	return events.APIGatewayProxyResponse{
		Body:       string(respBody),
		StatusCode: 200,
	}, nil
}

type JiraAuthSecretItem struct {
	JiraUsername string `json:"JiraUserName"`
	JiraApiKey   string `json:"JiraApiKey"`
}

func (svc *JiraService) GetSecretValues(SecretCredArn string) (JiraAuthSecretItem, error) {

	var secretData JiraAuthSecretItem

	if SecretCredArn == "" {
		return secretData, fmt.Errorf("[ERROR] Secret Arn : %v cannot be empty ", SecretCredArn)
	}

	input := secretsmanager.GetSecretValueInput{
		SecretId: aws.String(SecretCredArn),
	}

	svc.logger.Printf("Input : %v", input)

	output, err := svc.secretMgrClient.GetSecretValue(svc.ctx, &input)
	if err != nil {
		svc.logger.Printf("[ERROR] Failed to retrieve data from the secret manager. Error : %v", err)
		return secretData, err
	}

	err = json.Unmarshal([]byte(*output.SecretString), &secretData)
	if err != nil {
		svc.logger.Printf("[ERROR] Failed to Unmarshal Secret Data. Error : %v", err)
		return secretData, err
	}

	if secretData.JiraApiKey == "" {
		svc.logger.Printf("[ERROR] Secret Key : %v cannot be empty  ", secretData.JiraApiKey)
		return secretData, fmt.Errorf("[ERROR] Secret Key : %v cannot be empty ", secretData.JiraApiKey)
	}

	return secretData, nil
}

func (svc *JiraService) GetKBData(query string) (string, error) {

	output, err := svc.bedrockAgentClient.RetrieveAndGenerate(svc.ctx, &bedrockagentruntime.RetrieveAndGenerateInput{
		Input: &bedrockagenttypes.RetrieveAndGenerateInput{
			Text: aws.String(query),
		},
		RetrieveAndGenerateConfiguration: &bedrockagenttypes.RetrieveAndGenerateConfiguration{
			Type: bedrockagenttypes.RetrieveAndGenerateTypeKnowledgeBase,
			KnowledgeBaseConfiguration: &bedrockagenttypes.KnowledgeBaseRetrieveAndGenerateConfiguration{
				KnowledgeBaseId: aws.String(svc.KB_ID),
				ModelArn:        aws.String(BASE_MODEL_ID),
				OrchestrationConfiguration: &bedrockagenttypes.OrchestrationConfiguration{
					QueryTransformationConfiguration: &bedrockagenttypes.QueryTransformationConfiguration{
						Type: bedrockagenttypes.QueryTransformationTypeQueryDecomposition,
					},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}

	return *output.Output.Text, nil
}

// Each model provider has their own individual request and response formats.
// For the format, ranges, and default values for Amazon Titan Text, refer to:
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-titan-text.html
type TitanTextRequest struct {
	InputText            string               `json:"inputText"`
	TextGenerationConfig TextGenerationConfig `json:"textGenerationConfig"`
}

type TextGenerationConfig struct {
	Temperature   float64  `json:"temperature"`
	TopP          float64  `json:"topP"`
	MaxTokenCount int      `json:"maxTokenCount"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

type TitanTextResponse struct {
	InputTextTokenCount int      `json:"inputTextTokenCount"`
	Results             []Result `json:"results"`
}

type Result struct {
	TokenCount       int    `json:"tokenCount"`
	OutputText       string `json:"outputText"`
	CompletionReason string `json:"completionReason"`
}

func (svc JiraService) InvokeTitanText(prompt string) (string, error) {
	modelId := "amazon.titan-text-express-v1"

	svc.logger.Printf("Input Prompt :%v", prompt)

	body, err := json.Marshal(TitanTextRequest{
		InputText: prompt,
		TextGenerationConfig: TextGenerationConfig{
			Temperature:   0,
			TopP:          1,
			MaxTokenCount: 8000,
		},
	})

	if err != nil {
		log.Fatal("failed to marshal", err)
	}

	output, err := svc.bedrockClient.InvokeModel(context.Background(), &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelId),
		ContentType: aws.String("application/json"),
		Body:        body,
	})

	if err != nil {
		return "", err
	}

	var response TitanTextResponse
	if err := json.Unmarshal(output.Body, &response); err != nil {
		log.Fatal("failed to unmarshal", err)
	}

	return response.Results[0].OutputText, nil
}
