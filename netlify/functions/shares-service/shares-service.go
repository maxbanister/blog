package main

import (
	"context"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/maxbanister/blog/ap"
	. "github.com/maxbanister/blog/util"
)

func main() {
	lambda.Start(handleSharesService)
}

func handleSharesService(ctx context.Context, request LambdaRequest) (*LambdaResponse, error) {
	HOST_SITE := GetHostSite(ctx)

	return ap.FetchCol(&request, HOST_SITE, "shares")
}
