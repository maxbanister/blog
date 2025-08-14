package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/url"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/maxbanister/blog/netlify/ap"
	"github.com/maxbanister/blog/netlify/kv"
	. "github.com/maxbanister/blog/netlify/util"
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, request LambdaRequest) (*LambdaResponse, error) {
	authHdr := []byte(request.Headers["authorization"])
	selfAPIKey := []byte(os.Getenv("SELF_API_KEY"))

	if subtle.ConstantTimeCompare(authHdr, selfAPIKey) == 0 {
		fmt.Println("Authorization header did not match key")
		return &events.APIGatewayProxyResponse{StatusCode: 400}, nil
	}

	// QueryStringParameters automatically URL decodes these
	iconURL := request.QueryStringParameters["iconURL"]
	colName := request.QueryStringParameters["colName"]
	refID := request.QueryStringParameters["refID"]

	// get old actor
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return GetErrorResp(
			fmt.Errorf("could not start firestore client: %w", err),
		)
	}
	defer client.Close()

	parsedURI, err := url.Parse(refID)
	if err != nil {
		return GetErrorResp(fmt.Errorf("could not parse as URI: %w", err))
	}
	sluggedRef := Sluggify(*parsedURI)
	docSnap, err := client.Collection(colName).Doc(sluggedRef).Get(ctx)
	if err != nil {
		return GetErrorResp(fmt.Errorf("could not get doc: %w", err))
	}
	var docObj struct {
		Actor ap.Actor
	}
	err = docSnap.DataTo(&docObj)
	if err != nil {
		return GetErrorResp(fmt.Errorf("could not get actor: %w", err))
	}

	if docObj.Actor.Icon != iconURL {
		return GetErrorResp(
			fmt.Errorf("provided icon URL does not match actor icon"),
		)
	}

	// fetch the new actor profile
	newActor, err := ap.FetchActorAuthorized(docObj.Actor.Id)
	if err != nil {
		return GetErrorResp(
			fmt.Errorf("couldn't fetch actor's profile: %w", err),
		)
	}

	// update firestore's view of the actor
	err = kv.UpdateAllActorRefs(newActor)
	if err != nil {
		return GetErrorResp(
			fmt.Errorf("unable to update actor's profile: %w", err),
		)
	}

	iconURL, ok := newActor.Icon.(string)
	if !ok {
		return GetErrorResp(fmt.Errorf("actor icon wasn't string"))
	}

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       iconURL,
	}, nil
}
