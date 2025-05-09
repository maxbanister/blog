package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/maxbanister/blog/netlify/kv"
	. "github.com/maxbanister/blog/netlify/util"
	"google.golang.org/api/iterator"
)

func main() {
	lambda.Start(handleFollowers)
}

func handleFollowers(request LambdaRequest) (*LambdaResponse, error) {
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		fmt.Println("could not start firestore client:", err)
		return &events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       err.Error(),
		}, nil
	}
	defer client.Close()

	var followers []string

	iter := client.Collection("followers").Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			fmt.Println("could not call iter next on collection:", err)
			return &events.APIGatewayProxyResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       err.Error(),
			}, nil
		}
		followers = append(followers, doc.Data()["Id"].(string))
	}

	fmt.Printf("%v\n", followers)
	host := GetHostSite()

	payloadStr := strings.Builder{}
	payloadStr.WriteString(`{
	"@context": "https://www.w3.org/ns/activitystreams",
	"id": "`)
	payloadStr.WriteString(host)
	payloadStr.WriteString(`/ap/followers",
	"type": "OrderedCollection",
	"totalItems": `)
	payloadStr.WriteString(strconv.Itoa(len(followers)))
	payloadStr.WriteString(`,
	"orderedItems": `)
	followersJSON, _ := json.MarshalIndent(followers, "	", "	")
	payloadStr.Write(followersJSON)
	payloadStr.WriteString("\n}")

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/activity+json"},
		Body:       payloadStr.String(),
	}, nil
}
