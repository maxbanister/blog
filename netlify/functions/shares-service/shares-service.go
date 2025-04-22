package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	. "github.com/maxbanister/blog/util"
)

func main() {
	lambda.Start(handleInbox)
}

func handleInbox(ctx context.Context, request LambdaRequest) (*LambdaResponse, error) {
	fmt.Println(request.Headers)

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "Success",
	}, nil
}
