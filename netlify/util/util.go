package util

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

var ErrUnauthorized = errors.New(http.StatusText(http.StatusUnauthorized))
var ErrNotImplemented = errors.New(http.StatusText(http.StatusNotImplemented))
var ErrBadRequest = errors.New(http.StatusText(http.StatusBadRequest))

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
	return netlifyData["site_url"]
}
