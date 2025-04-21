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
		return GetLambdaResp(CallFollowService(&request, HOST_SITE, actorObj))

	case "Create":
		return GetLambdaResp(HandleReply(&request, requestJSON, HOST_SITE))

	case "Undo":
		object, ok := requestJSON["object"].(map[string]any)
		if !ok || object["type"] != "Follow" {
			break
		}
		return GetLambdaResp(HandleUnfollow(&request, requestJSON))

	case "Delete":
		return GetLambdaResp(HandleDelete(&request, requestJSON))

	case "Update":
		object, _ := requestJSON["object"].(map[string]any)
		var err error
		if object["type"] == "Person" {
			err = HandleProfileUpdate(&request, requestJSON)
		} else if object["type"] == "Note" {
			err = HandleReplyEdit(&request, requestJSON)
		} else {
			break
		}
		return GetLambdaResp(err)
	}

	return GetLambdaResp(fmt.Errorf(
		"%w: unsupported operation", ErrNotImplemented))
}
