package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/maxbanister/blog/netlify/ap"
	"github.com/maxbanister/blog/netlify/kv"
	. "github.com/maxbanister/blog/netlify/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, request LambdaRequest) (*LambdaResponse, error) {
	host := GetHostSite()
	// extract the referred to post from the query parameters
	postID := request.QueryStringParameters["id"]
	fmt.Println("Got request for", postID)

	// if accept is of type application/ld+json or /activity+json, return only
	// shallow replies with external reference IDs to reply objects
	wantsAP := false
	a := strings.ToLower(request.Headers["accept"])
	if strings.Contains(a, "activity+json") || strings.Contains(a, "ld+json") {
		wantsAP = true
	}

	client, err := kv.GetFirestoreClient()
	if err != nil {
		return nil, fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	postURIString := host + "/posts/" + postID
	r, err := GetReplyTree(client, postURIString, wantsAP)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// dummy object that has no replies
			r = &ap.Reply{Replies: ap.InnerReplies{
				Id: postURIString + "/replies",
			}}
		} else {
			return nil, err
		}
	}

	if !wantsAP {
		body, err := json.Marshal(r.Replies)
		if err != nil {
			return nil, fmt.Errorf("couldn't marshal reply tree json: %w", err)
		}
		return &events.APIGatewayProxyResponse{
			StatusCode: 200,
			Headers: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
			},
			Body: string(body),
		}, nil
	}

	// Simple one-deep tree for ActivityPub compliance
	replyItems, _ := json.MarshalIndent(r.Replies.Items, "	", "	")
	body := fmt.Sprintf(`{
	"@context": "https://www.w3.org/ns/activitystreams",
	"id": "%s",
	"type": "OrderedCollection",
	"totalItems": %d,
	"items": %s
}`, r.Replies.Id, len(r.Replies.Items), string(replyItems))

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/activity+json",
		},
		Body: body,
	}, nil
}

func GetReplyTree(client *firestore.Client, replyURI string, shallow bool) (*ap.Reply, error) {
	replyID, err := url.Parse(replyURI)
	if err != nil {
		return nil, fmt.Errorf("could not parse as URI: %w", err)
	}
	slugPostID := Sluggify(*replyID)

	ctx := context.Background()
	replyDoc, err := client.Collection("replies").Doc(slugPostID).Get(ctx)
	if err != nil {
		return nil, err
	}

	var r ap.Reply
	err = replyDoc.DataTo(&r)
	if err != nil {
		return nil, fmt.Errorf("couldn't populate struct with doc: %w", err)
	}
	if shallow {
		return &r, nil
	}

	for i, item := range r.Replies.Items {
		itemStr, ok := item.(string)
		if !ok {
			fmt.Println("warning: linked reply item is not string:", item)
			continue
		}
		subTree, err := GetReplyTree(client, itemStr, false)
		if err != nil {
			return nil, err
		}
		r.Replies.Items[i] = subTree
	}

	return &r, nil
}
