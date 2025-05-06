package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/maxbanister/blog/netlify/ap"
	. "github.com/maxbanister/blog/netlify/util"
)

func main() {
	lambda.Start(handleInbox)
}

func handleInbox(ctx context.Context, request LambdaRequest) (*LambdaResponse, error) {
	HOST_SITE := GetHostSite()
	fmt.Println("Headers:", request.Headers)
	fmt.Println("Body:", request.Body)

	requestJSON := make(map[string]any)
	err := json.Unmarshal([]byte(request.Body), &requestJSON)
	if err != nil {
		return GetLambdaResp(fmt.Errorf(
			"%w: bad json syntax: %s", ErrBadRequest, err.Error()))
	}
	actor, err := ap.RecvActivity(&request, requestJSON)
	if err != nil {
		return nil, err
	}

	fmt.Println("Request type:", requestJSON["type"])

	switch requestJSON["type"] {
	case "Follow":
		err := HandleFollow(actor, requestJSON)
		if err != nil {
			return GetLambdaResp(err)
		}
		return GetLambdaResp(CallFollowService(&request, HOST_SITE, actor))

	case "Create":
		err := HandleReply(&request, actor, requestJSON, HOST_SITE)
		return GetLambdaResp(err)

	case "Undo":
		if object, ok := requestJSON["object"].(map[string]any); ok {
			if object["type"] == "Follow" {
				return GetLambdaResp(HandleUnfollow(actor, requestJSON))
			} else if object["type"] == "Like" {
				return GetLambdaResp(HandleUnlike(requestJSON))
			} else if object["type"] == "Announce" {
				return GetLambdaResp(HandleUnannounce(requestJSON))
			}
		} else if objectStr, ok := requestJSON["object"].(string); ok {
			if strings.Contains(objectStr, "app.bsky.feed.like") {
				return GetLambdaResp(HandleUnfollow(actor, requestJSON))
			} else if strings.Contains(objectStr, "app.bsky.feed.repost") {
				return GetLambdaResp(HandleUnannounce(requestJSON))
			}
		}

	case "Delete":
		return GetLambdaResp(HandleDelete(requestJSON))

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

	case "Like":
		return GetLambdaResp(HandleLike(actor, requestJSON, HOST_SITE))

	case "Announce":
		return GetLambdaResp(HandleAnnounce(actor, requestJSON, HOST_SITE))

	case "Accept":
		object, _ := requestJSON["object"].(map[string]any)
		fmt.Println("Got AcceptFollow from", requestJSON["actor"])
		if object["type"] == "Follow" {
			return GetLambdaResp(nil)
		} else {
			return GetLambdaResp(fmt.Errorf("%w: only accepts follow requests",
				ErrBadRequest))
		}
	}

	return GetLambdaResp(fmt.Errorf(
		"%w: unsupported operation", ErrNotImplemented))
}
