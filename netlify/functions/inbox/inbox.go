package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	. "github.com/maxbanister/blog/ap"
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

		actorBytes, err := json.Marshal(&actorObj)
		if err != nil {
			return getLambdaResp(fmt.Errorf(
				"%w: could not encode actor string: %w", ErrBadRequest, err))
		}
		replyReq := ReplyServiceRequest{
			ReplyObj: request.Body,
			Actor:    actorBytes,
		}
		reqBody, err := json.Marshal(replyReq)
		if err != nil {
			return getLambdaResp(fmt.Errorf(
				"%w: could not encode reply request: %w", ErrBadRequest, err))
		}

		// fire and forget
		go func() {
			url := HOST_SITE + "/ap/reply-service"
			req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
			if err != nil {
				fmt.Println("could not form request:", err)
				return
			}
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
			req.Header.Set("Authorization", os.Getenv("SELF_API_KEY"))

			fmt.Println("Req:", req.Body)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				fmt.Println("could not send post to reply service:", err)
				return
			}
			fmt.Println("Resp:", resp, "Err:", err)
		}()

		// give the reply service a chance to read the request
		time.Sleep(100 * time.Millisecond)

		return getLambdaResp(nil)
	default:
		return getLambdaResp(fmt.Errorf(
			"%w: unsupported operation", ErrNotImplemented))
	}
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
