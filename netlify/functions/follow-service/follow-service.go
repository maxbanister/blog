package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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

	var followReq FollowServiceRequest
	err := json.Unmarshal([]byte(request.Body), &followReq)
	if err != nil {
		fmt.Println("could not unmarshal json:", err)
		return &events.APIGatewayProxyResponse{StatusCode: 400}, nil
	}

	followObj := followReq.FollowObj

	var actor Actor
	err = json.Unmarshal(followReq.Actor, &actor)
	if err != nil {
		fmt.Println("could not unmarshal actor:", err)
		return &events.APIGatewayProxyResponse{StatusCode: 400}, nil
	}

	hostSite := GetHostSite(ctx)
	AcceptRequest(hostSite, followObj, &actor)

	return &events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func AcceptRequest(hostSite, followReqBody string, actor *Actor) {
	// Pre-validated actor name and inbox
	parsedURL, _ := url.Parse(actor.Id)
	actorAt := actor.Name + "@" + parsedURL.Host
	fmt.Println("Actor:", actorAt)

	payload := fmt.Sprintf(`{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id": "%s/ap/user/blog#accepts/follows/%s",
		"type": "Accept",
		"actor": "https://%s/ap/user/blog",
		"object": %s%s`, hostSite, actorAt, hostSite, followReqBody, "\n}\n")

	SendActivity(payload, actor)
}
