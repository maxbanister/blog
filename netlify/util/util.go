package util

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

var ErrUnauthorized = errors.New(http.StatusText(http.StatusUnauthorized))
var ErrNotImplemented = errors.New(http.StatusText(http.StatusNotImplemented))
var ErrBadRequest = errors.New(http.StatusText(http.StatusBadRequest))
var ErrAlreadyDone = errors.New("already done")

type LambdaRequest = events.APIGatewayProxyRequest
type LambdaResponse = events.APIGatewayProxyResponse

func GetHostSite(ctx context.Context) string {
	var jsonData []byte
	var err error
	lc, ok := lambdacontext.FromContext(ctx)
	if !ok {
		fmt.Println("could not get lambda context")
	} else {
		ccc := lc.ClientContext.Custom
		jsonData, err = base64.StdEncoding.DecodeString(ccc["netlify"])
		if err != nil {
			fmt.Println("could not decode netlify base64:", err)
			return ""
		}
	}
	var netlifyData map[string]string
	err = json.Unmarshal(jsonData, &netlifyData)
	if err != nil {
		fmt.Println("could not decode netlify json:", err)
		return ""
	}
	return strings.TrimPrefix(netlifyData["site_url"], "https://")
}

func Sluggify(uri url.URL) string {
	uri.Fragment = ""
	uri.Scheme = ""
	uriStr := strings.ToLower(uri.String()[2:])
	var res strings.Builder
	lastDash := false
	for _, c := range uriStr {
		if strings.ContainsRune("/@-.:#", c) {
			if !lastDash {
				res.WriteRune('-')
				lastDash = true
			}
		} else {
			res.WriteRune(c)
			lastDash = false
		}
	}
	return strings.Trim(res.String(), "-")
}

func GetLambdaResp(err error) (*LambdaResponse, error) {
	var code int
	if errors.Is(err, ErrUnauthorized) {
		code = http.StatusUnauthorized
	} else if errors.Is(err, ErrBadRequest) {
		code = http.StatusBadRequest
	} else if errors.Is(err, ErrNotImplemented) {
		code = http.StatusNotImplemented
	} else if err != nil {
		code = http.StatusInternalServerError
	} else if errors.Is(err, ErrAlreadyDone) {
		code = http.StatusAlreadyReported
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
