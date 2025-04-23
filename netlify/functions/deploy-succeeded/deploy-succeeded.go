package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	. "github.com/maxbanister/blog/util"
)

func main() {
	lambda.Start(handleDeploy)
}

func handleDeploy(ctx context.Context, request LambdaRequest) (*LambdaResponse, error) {
	fmt.Println("here lol")

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "ok",
	}, nil
}
