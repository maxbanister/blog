package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
	. "github.com/maxbanister/blog/util"
)

func main() {
	lambda.Start(handleInbox)
}

func handleInbox(ctx context.Context, request LambdaRequest) (*LambdaResponse, error) {
	HOST_SITE := GetHostSite(ctx)
	fmt.Println("Headers:", request.Headers)
	fmt.Println("Body:", request.Body)

	requestJSON := make(map[string]any)
	err := json.Unmarshal([]byte(request.Body), &requestJSON)
	if err != nil {
		return GetLambdaResp(fmt.Errorf(
			"%w: bad json syntax: %s", ErrBadRequest, err.Error()))
	}

	fmt.Println("Request type:", requestJSON["type"])

	switch requestJSON["type"] {
	case "Follow":
		actorObj, err := HandleFollow(&request, requestJSON)
		if err != nil {
			return GetLambdaResp(err)
		}

		if err := CallFollowService(&request, HOST_SITE, actorObj); err != nil {
			return GetLambdaResp(err)
		}

		return GetLambdaResp(nil)

	case "Create":
		err = HandleReply(&request, requestJSON, HOST_SITE)
		if err != nil {
			return GetLambdaResp(err)
		}

		return GetLambdaResp(nil)

	case "Undo":
		var err error
		object, ok := requestJSON["object"].(map[string]any)
		if !ok || object["type"] != "Follow" {
			break
		}

		err = HandleUnfollow(&request, requestJSON)
		if err != nil {
			return GetLambdaResp(err)
		}

		return GetLambdaResp(nil)

	case "Delete":
		err = HandleDelete(&request, requestJSON)
		if err != nil {
			return GetLambdaResp(err)
		}

		return GetLambdaResp(nil)
	}

	return GetLambdaResp(fmt.Errorf(
		"%w: unsupported operation", ErrNotImplemented))
}
