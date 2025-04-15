package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	. "github.com/maxbanister/blog/ap"
	"github.com/maxbanister/blog/kv"
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
		return getLambdaResp(fmt.Errorf(
			"%w: bad json syntax: %s", ErrBadRequest, err.Error()))
	}

	switch requestJSON["type"] {
	case "Follow":
		actorObj, err := HandleFollow(&request, requestJSON)
		if err != nil {
			return getLambdaResp(err)
		}

		if err := CallFollowService(&request, HOST_SITE, actorObj); err != nil {
			return getLambdaResp(err)
		}

		return getLambdaResp(nil)

	case "Undo":
		var err error
		object, ok := requestJSON["object"].(map[string]any)
		if !ok || object["type"] != "Follow" {
			goto unsupported
		}

		err = HandleUnfollow(&request, requestJSON)
		if err != nil {
			return getLambdaResp(err)
		}

		return getLambdaResp(nil)

	unsupported:
		fallthrough

	default:
		return getLambdaResp(fmt.Errorf(
			"%w: unsupported operation", ErrNotImplemented))
	}
}

func HandleFollow(r *LambdaRequest, requestJSON map[string]any) (*Actor, error) {
	actor, err := RecvActivity(r, requestJSON)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return nil, fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	// write to json database
	actorAt := GetActorAt(actor)
	_, err = client.Collection("followers").Doc(actorAt).Set(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("failed adding follower: %v", err)
	}

	return actor, nil
}

func HandleUnfollow(r *LambdaRequest, requestJSON map[string]any) error {
	actor, err := RecvActivity(r, requestJSON)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	// write to json database
	actorAt := GetActorAt(actor)
	_, err = client.Collection("followers").Doc(actorAt).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove follower: %v", err)
	}

	return nil
}

// Invokes the serverless function to send an AcceptFollow to the actor's inbox
func CallFollowService(request *LambdaRequest, hostSite string, actorObj *Actor) error {
	actorBytes, err := json.Marshal(actorObj)
	if err != nil {
		return fmt.Errorf("%w: could not encode actor string: %w",
			ErrBadRequest, err)
	}
	followReq := FollowServiceRequest{
		FollowObj: request.Body,
		Actor:     actorBytes,
	}
	reqBody, err := json.Marshal(followReq)
	if err != nil {
		return fmt.Errorf("%w: could not encode reply request: %w",
			ErrBadRequest, err)
	}

	fmt.Println("spawning goroutine")

	// fire and forget
	go func() {
		url := hostSite + "/ap/follow-service"
		req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
		if err != nil {
			fmt.Println("could not form request:", err)
			return
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Authorization", os.Getenv("SELF_API_KEY"))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println("could not send post to follow service:", err)
			return
		}
		fmt.Println("Resp:", resp, "Err:", err)
	}()

	fmt.Println("after goroutine")

	return nil
}

func getLambdaResp(err error) (*LambdaResponse, error) {
	var code int
	if errors.Is(err, ErrUnauthorized) {
		code = http.StatusUnauthorized
	} else if errors.Is(err, ErrBadRequest) {
		code = http.StatusBadRequest
	} else if errors.Is(err, ErrNotImplemented) {
		code = http.StatusNotImplemented
	} else {
		code = http.StatusOK
	}

	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}
	fmt.Println(code, err)
	return &events.APIGatewayProxyResponse{
		StatusCode: code,
		Body:       errMsg,
	}, nil
}
