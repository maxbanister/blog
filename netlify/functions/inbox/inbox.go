package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	. "github.com/maxbanister/blog/ap"
)

func main() {
	lambda.Start(handleInbox)
}

func handleInbox(request LambdaRequest) (*LambdaResponse, error) {
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

		url := "https://maxscribes.netlify.app/.netlify/functions/async-workloads-router?events=say-hello"
		req, err := http.NewRequest("POST", url, strings.NewReader(
			fmt.Sprintf(`{
				eventName: "%s",
				data: "%s"
			}`, "say-hello", ""),
		))
		req.Header.Set("Authorization", os.Getenv("AWL_API_KEY_P10"))
		fmt.Println("Req:", req)
		if err != nil {
			fmt.Println("Err:", err)
		} else {
			resp, err := (&http.Client{}).Do(req)
			fmt.Println("Resp:", resp, "Err:", err)
		}

		// fire and forget
		go AcceptRequest(request.Body, actorObj)
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
