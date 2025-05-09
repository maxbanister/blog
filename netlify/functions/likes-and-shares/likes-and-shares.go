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
	lambda.Start(handleService)
}

func handleService(ctx context.Context, request LambdaRequest) (*LambdaResponse, error) {
	HOST_SITE := GetHostSite()

	colName := request.QueryStringParameters["col"]
	if colName != "likes" && colName != "shares" {
		return GetErrorResp(
			fmt.Errorf("unallowed collection name: %s", colName),
		)
	}

	return FetchCol(&request, HOST_SITE, colName)
}

func FetchCol(r *LambdaRequest, host, colName string) (*LambdaResponse, error) {
	// connect to firestore database
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return GetErrorResp(
			fmt.Errorf("could not start firestore client: %w", err),
		)
	}
	defer client.Close()

	// get title from query param
	postID := r.QueryStringParameters["id"]
	postURIString := host + "/posts/" + postID
	fmt.Println(postURIString)

	// form full sluggified url
	postURI, err := url.Parse(postURIString)
	if err != nil {
		return nil, fmt.Errorf("could not parse as URI: %w", err)
	}
	slugPostURI := Sluggify(*postURI)
	fmt.Println("Got request for", slugPostURI)

	wantsAP := false
	a := strings.ToLower(r.Headers["accept"])
	if strings.Contains(a, "activity+json") || strings.Contains(a, "ld+json") {
		wantsAP = true
	}

	// get top-level likes document from firestore
	collectionRef := client.Collection(colName)
	ctx := context.Background()
	doc, err := collectionRef.Doc(slugPostURI).Get(ctx)
	docExists := true
	if err != nil {
		if status.Code(err) == codes.NotFound {
			docExists = false
		} else {
			return GetErrorResp(
				fmt.Errorf("could not get top-level doc: %w", err),
			)
		}
	}
	var items any
	if docExists {
		items, err = doc.DataAt("Items")
		if err != nil {
			return GetErrorResp(fmt.Errorf("could not get items: %w", err))
		}
	}
	activityURIs, _ := items.([]any)
	var docRefs []*firestore.DocumentRef
	for _, uri := range activityURIs {
		uriString, ok := uri.(string)
		if !ok {
			continue
		}
		parsedURI, err := url.Parse(uriString)
		if err != nil {
			fmt.Println("could not parse URI", uri)
			continue
		}
		docTitle := Sluggify(*parsedURI)
		fmt.Println("adding", docTitle)
		docRefs = append(docRefs, collectionRef.Doc(docTitle))
	}

	docs, err := client.GetAll(ctx, docRefs)
	if err != nil {
		return GetErrorResp(fmt.Errorf("could not GetAll %s: %w", colName, err))
	}

	likesOrShares := []*ap.LikeOrShare{}
	for _, doc := range docs {
		likeOrShare := &ap.LikeOrShare{}
		err := doc.DataTo(&likeOrShare)
		if err != nil {
			fmt.Println("could not convert activity doc to struct:", err)
			continue
		}
		fmt.Println(likeOrShare)
		likesOrShares = append(likesOrShares, likeOrShare)
	}

	// format pseudo-AP response with all items
	if !wantsAP {
		respBody, err := json.Marshal(likesOrShares)
		if err != nil {
			return GetErrorResp(fmt.Errorf("could not marshal slice: %w", err))
		}
		return &events.APIGatewayProxyResponse{
			StatusCode: 200,
			Headers: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
			},
			Body: string(respBody),
		}, nil
	}

	// Like activities aren't dereferenceable with Masto, so we must include the
	// full object. With Announces, we can simply reference the ID
	lst := make([]any, len(likesOrShares))
	for i, likeOrShare := range likesOrShares {
		if colName == "likes" {
			lst[i] = map[string]string{
				"id":     likeOrShare.Id,
				"type":   "Like",
				"actor":  likeOrShare.Actor.Id,
				"object": likeOrShare.Object,
			}
		} else { // colName == "shares"
			lst[i] = likeOrShare.Id
		}
	}

	lstBytes, _ := json.MarshalIndent(lst, "	", "	")
	body := fmt.Sprintf(`{
	"@context": "https://www.w3.org/ns/activitystreams",
	"id": "%s",
	"type": "Collection",
	"totalItems": %d,
	"items": %s
}`, postURIString+"/"+colName, len(lst), string(lstBytes))

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/activity+json",
		},
		Body: body,
	}, nil
}
