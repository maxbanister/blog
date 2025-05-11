package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

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
	fmt.Println("Deploy successful")

	var outbox struct {
		OrderedItems []json.RawMessage `json:"orderedItems"`
	}
	err := json.Unmarshal(blog.OutboxJSON, &outbox)
	if err != nil {
		return GetErrorResp(fmt.Errorf("could not decode outbox JSON: %w", err))
	}

	type OutboxItem struct {
		Typ     string `json:"type"`
		ID      string `json:"id"`
		Payload string `json:"-"`
	}
	var validOutboxItems []OutboxItem
	topPost := false

	for _, outboxActivity := range outbox.OrderedItems {
		var decodedItem OutboxItem
		err := json.Unmarshal(outboxActivity, &decodedItem)
		if err != nil {
			fmt.Println("could not decode outbox item:", err.Error())
			continue
		}
		// Send out all the deletes every time
		if decodedItem.Typ == "Delete" || !topPost {
			fmt.Printf("Queuing %s of %s\n", decodedItem.Typ, decodedItem.ID)
			decodedItem.Payload = string(outboxActivity)
			fmt.Println(decodedItem.Payload)
			validOutboxItems = append(validOutboxItems, decodedItem)
			topPost = true
		}
	}

	if len(validOutboxItems) == 0 {
		return &events.APIGatewayProxyResponse{
			StatusCode: 200,
			Body:       "outbox empty",
		}, nil
	}

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

	var wg sync.WaitGroup

	// broadcast to followers
	fmt.Printf("Broadcasting %d changes...\n", len(validOutboxItems))
	for _, outboxItem := range validOutboxItems {
		for _, follower := range followers {
			// Bluesky doesn't support editing posts
			isBskyUsr := strings.HasPrefix(follower.Id, "https://bsky.brid.gy/")
			if outboxItem.Typ == "Update" && isBskyUsr {
				continue
			}

			wg.Add(1)

			go func(follower ap.Actor) {
				defer wg.Done()
				err = ap.SendActivity(outboxItem.Payload, &follower)
				if err != nil {
					fmt.Printf("failed to send %s to %s: %s\n", outboxItem.ID,
						follower.Id, err.Error())
				} else {
					fmt.Printf("successfully sent %s to %s\n", outboxItem.ID,
						follower.Id)
				}
			}(*follower)
		}
	}

	wg.Wait()

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "ok",
	}, nil
}
