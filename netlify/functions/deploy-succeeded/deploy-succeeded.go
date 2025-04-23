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
	HOST_SITE := GetHostSite()
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
	postBody := outbox.OrderedItems[0]

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

	var item struct {
		Id        string `json:"id"`
		Published string `json:"published"`
	}
	err = json.Unmarshal(postBody, &item)
	if err != nil || item.Id == "" {
		return GetErrorResp(fmt.Errorf("could not get id from post: %w", err))
	}
	postID := item.Id
	published := item.Published
	selfUser := HOST_SITE + "/ap/user/blog"
	followersURI := HOST_SITE + "/ap/followers"

	payload := fmt.Sprintf(`{
	"@context": "https://www.w3.org/ns/activitystreams",
	"id": "%s/create",
	"type": "Create",
	"actor": "%s",
	"published": "%s",
	"to": [
		"https://www.w3.org/ns/activitystreams#Public"
	],
	"cc": [
		"%s"
	],
	"object": %s
}`, postID, selfUser, published, followersURI, string(postBody))

	fmt.Println("Sending post", postID)

	// broadcast to followers
	for _, follower := range followers {
		err = ap.SendActivity(payload, follower)
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
