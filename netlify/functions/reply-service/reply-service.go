package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	. "github.com/maxbanister/blog/ap"
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, request LambdaRequest) (*LambdaResponse, error) {
	if request.Headers["authorization"] != os.Getenv("SELF_API_KEY") {
		fmt.Println("Authorization header did not match key")
		return &events.APIGatewayProxyResponse{StatusCode: 400}, nil
	}

	var replyReq ReplyServiceRequest
	err := json.Unmarshal([]byte(request.Body), &replyReq)
	if err != nil {
		fmt.Println("could not unmarshal json:", err)
		return &events.APIGatewayProxyResponse{StatusCode: 400}, nil
	}

	followObj := replyReq.ReplyObj

	var actor Actor
	err = json.Unmarshal(replyReq.Actor, &actor)
	if err != nil {
		fmt.Println("could not unmarshal actor:", err)
		return &events.APIGatewayProxyResponse{StatusCode: 400}, nil
	}

	hostSite := GetHostSite(ctx)
	AcceptRequest(hostSite, followObj, &actor)

	return &events.APIGatewayProxyResponse{StatusCode: 200}, nil
}
