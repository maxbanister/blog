package main

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/maxbanister/blog/netlify/ap"
	"github.com/maxbanister/blog/netlify/kv"
	. "github.com/maxbanister/blog/netlify/util"
)

func main() {
	lambda.Start(handleRefreshProfile)
}

func handleRefreshProfile(request LambdaRequest) (*LambdaResponse, error) {
	authHdr := []byte(request.Headers["authorization"])
	selfAPIKey := []byte(os.Getenv("SELF_API_KEY"))

	if subtle.ConstantTimeCompare(authHdr, selfAPIKey) == 0 {
		fmt.Println("Authorization header did not match key")
		return &events.APIGatewayProxyResponse{StatusCode: 400}, nil
	}

	actorID := request.QueryStringParameters["actorID"]

	// fetch the new actor profile
	actor, err := ap.FetchActorAuthorized(actorID)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch actor's profile: %w", err)
	}

	// update firestore's view of the actor
	err = kv.UpdateAllActorRefs(actor)
	if err != nil {
		fmt.Println("unable to update actor's profile: %w", err)
	}

	iconURL, ok := actor.Icon.(string)
	if !ok {
		return nil, errors.New("actor icon wasn't string")
	}

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       iconURL,
	}, nil
}
