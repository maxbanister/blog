package main

import (
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	. "github.com/maxbanister/blog/ap"
)

func main() {
	lambda.Start(handle)
}

func handle(request LambdaRequest) (*LambdaResponse, error) {

	fmt.Println("before sleep")
	time.Sleep(1 * time.Second)

	fmt.Println("after sleep")

	return &events.APIGatewayProxyResponse{
		StatusCode: 202,
		Body:       "Hello, World!",
	}, nil
}
