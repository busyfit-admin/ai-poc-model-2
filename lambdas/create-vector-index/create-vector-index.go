package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/cfn"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-xray-sdk-go/xray"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	requestsigner "github.com/opensearch-project/opensearch-go/v4/signer/awsv2"
)

type OpenSearchService struct {
	ctx              context.Context
	logger           *log.Logger
	openSearchClient *opensearchapi.Client

	OpenSearchEndpoint string

	IndexName    string
	IndexMapping string
}

func main() {
	ctx, root := xray.BeginSegment(context.TODO(), "create-vector-index")
	defer root.Close(nil)

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v\n", err)
	}

	// Create an AWS request Signer and load AWS configuration using default config folder or env vars.
	signer, err := requestsigner.NewSignerWithService(cfg, "aoss") // Use "aoss" for Amazon OpenSearch Serverless
	if err != nil {
		log.Fatalf("Cannot perform signer client : %v\n", err)
	}
	// Create an opensearch client and use the request-signer.
	client, err := opensearchapi.NewClient(
		opensearchapi.Config{
			Client: opensearch.Config{
				Addresses: []string{os.Getenv("OPEN_SEARCH_ENDPOINT")},
				Signer:    signer,
			},
		},
	)
	if err != nil {
		log.Fatalf("Cannot perform signer client : %v\n", err)
	}

	svc := OpenSearchService{
		ctx:              ctx,
		logger:           log.New(os.Stdout, "", log.LstdFlags),
		openSearchClient: client,
	}

	lambda.Start(cfn.LambdaWrap(svc.handler))

}

func (svc *OpenSearchService) handler(ctx context.Context, cfnEvent cfn.Event) (string, map[string]interface{}, error) {

	svc.ctx = ctx
	response := make(map[string]interface{})

	var err error

	bytes, _ := json.Marshal(cfnEvent)
	log.Printf("Handling event: %# v\n", string(bytes))

	err = svc.setup(cfnEvent)
	if err != nil {
		log.Printf("Could not set up to handle the event: %v\n", err)
		return "FAILED", response, err
	}

	switch cfnEvent.RequestType {
	case cfn.RequestCreate:
		return svc.CreateIndex()
	case cfn.RequestDelete:
		return svc.DeleteIndex()
	default:
		return "SUCCESS", response, nil
	}

}

func (svc *OpenSearchService) setup(event cfn.Event) error {

	svc.IndexName = event.ResourceProperties["IndexName"].(string)
	svc.IndexMapping = event.ResourceProperties["IndexMapping"].(string)

	return nil
}

func (svc *OpenSearchService) CreateIndex() (string, map[string]interface{}, error) {

	response := make(map[string]interface{})
	// Create an index with non-default settings.
	createResp, err := svc.openSearchClient.Indices.Create(
		svc.ctx,
		opensearchapi.IndicesCreateReq{
			Index: svc.IndexName,
			Body:  strings.NewReader(svc.IndexMapping),
		},
	)
	if err != nil {
		return "FAILED", response, err
	}

	svc.logger.Printf("Index Created :%v", createResp)

	return "SUCCESS", response, nil
}

func (svc *OpenSearchService) DeleteIndex() (string, map[string]interface{}, error) {
	response := make(map[string]interface{})

	delResp, err := svc.openSearchClient.Indices.Delete(svc.ctx, opensearchapi.IndicesDeleteReq{Indices: []string{svc.IndexName}})
	if err != nil {
		return "FAILED", response, err
	}

	svc.logger.Printf("deleted index: %v", delResp.Acknowledged)
	return "SUCCESS", response, nil
}
