package util

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

var ErrUnauthorized = errors.New(http.StatusText(http.StatusUnauthorized))
var ErrNotImplemented = errors.New(http.StatusText(http.StatusNotImplemented))
var ErrBadRequest = errors.New(http.StatusText(http.StatusBadRequest))
var ErrAlreadyDone = errors.New("already done")

type LambdaRequest = events.APIGatewayProxyRequest
type LambdaResponse = events.APIGatewayProxyResponse

func GetHostSite() string {
	host := os.Getenv("URL")
	// hack to get to work in dev
	if host == "" || strings.Contains(host, "localhost") {
		return "https://maxbanister.com"
	} else {
		return host
	}
}

func Sluggify(uri url.URL) string {
	uri.Scheme = ""
	uriStr := strings.ToLower(uri.String())
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
	} else if errors.Is(err, ErrAlreadyDone) {
		code = http.StatusAlreadyReported
	} else if err != nil {
		code = http.StatusInternalServerError
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

// Simpler version of the above that only returns status code 500
func GetErrorResp(err error) (*LambdaResponse, error) {
	return &events.APIGatewayProxyResponse{
		StatusCode: 500,
		Body:       err.Error(),
	}, nil
}
