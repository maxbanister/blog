package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/maxbanister/blog/kv"
	. "github.com/maxbanister/blog/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func main() {
	lambda.Start(handle)
}

func handle(request LambdaRequest) (*LambdaResponse, error) {
	// get GET request with query parameter (?) of the url to get the replies of

	// extract the url from the query parameters
	postID := request.QueryStringParameters["id"]

	postURI, err := url.Parse(postID)
	if err != nil {
		return &events.APIGatewayProxyResponse{
			StatusCode: 400,
			Body:       fmt.Sprintf("post ID is invalid URI: %s", err.Error()),
		}, nil
	}

	// normalize url using sluggify
	slugPostID := Sluggify(*postURI)

	// fetch top-level replies object from firestore
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return nil, fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	replyDoc, err := client.Collection("replies").Doc(slugPostID).Get(ctx)
	if err != nil {
		if status.Code(err) != codes.NotFound {
			return nil, fmt.Errorf("error looking up replies: %w", err)
		}
		return &events.APIGatewayProxyResponse{
			StatusCode: 404,
			Body:       "no replies yet",
		}, nil
	}
	replyID, _ := replyDoc.DataAt("Replies.Id")
	replyItems, _ := replyDoc.DataAt("Replies.Items")
	replyItemsArr, _ := replyItems.([]string)
	replyItemsBytes, _ := json.MarshalIndent(replyItems, "", "	")

	// STOP - if accept is of type application/ld+json, return only shallow replies with
	// external references to Id's
	a := request.Headers["accept"]
	if strings.Contains(a, "activity+json") || strings.Contains(a, "ld+json") {
		// @context
		// id
		// type: OrderedCollection
		// items: []string
		body := fmt.Sprintf(`{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id": "%s",
		"type": "OrderedCollection",
		"totalItems": %d,
		"items": %s
		}`, replyID, len(replyItemsArr), string(replyItemsBytes))

		return &events.APIGatewayProxyResponse{StatusCode: 200, Body: body}, nil
	}

	// recurse over Replies.Items

	// for each item, do another lookup in firestore for that sluggified URL

	// build up full replies tree using the Reply object

	// serialize and send to the requester

	return nil, nil
}
