package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	blog "github.com/maxbanister/blog"
	"github.com/maxbanister/blog/netlify/ap"
	"github.com/maxbanister/blog/netlify/kv"
	. "github.com/maxbanister/blog/netlify/util"
	"google.golang.org/api/iterator"
)

func main() {
	lambda.Start(handleDeploy)
}

func handleDeploy(request LambdaRequest) (*LambdaResponse, error) {
	fmt.Println("Broadcasting new post...")

	var outbox struct {
		OrderedItems []json.RawMessage `json:"orderedItems"`
	}
	err := json.Unmarshal(blog.OutboxJSON, &outbox)
	if err != nil {
		return GetErrorResp(fmt.Errorf("could not decode outbox JSON: %w", err))
	}

	if len(outbox.OrderedItems) == 0 {
		return &events.APIGatewayProxyResponse{
			StatusCode: 200,
			Body:       "no posts in outbox",
		}, nil
	}
	createActivity := outbox.OrderedItems[0]

	// get followers from firestore
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return GetErrorResp(
			fmt.Errorf("could not start firestore client: %w", err),
		)
	}
	defer client.Close()

	var followers []*ap.Actor
	iter := client.Collection("followers").Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return GetErrorResp(
				fmt.Errorf("could not call iter next on collection: %w", err),
			)
		}
		var follower ap.Actor
		err = doc.DataTo(&follower)
		if err != nil {
			return GetErrorResp(
				fmt.Errorf("could not convert doc to Actor: %w", err),
			)
		}
		followers = append(followers, &follower)
	}

	// broadcast to followers
	for _, follower := range followers {
		err = ap.SendActivity(string(createActivity), follower)
		if err != nil {
			fmt.Printf("failed sending post to %s: %s\n", follower.Id,
				err.Error())
		} else {
			fmt.Printf("successfully sent to %s\n", follower.Id)
		}
	}

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "ok",
	}, nil
}
