package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/maxbanister/blog/ap"
	"github.com/maxbanister/blog/kv"
	. "github.com/maxbanister/blog/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func main() {
	lambda.Start(handleInbox)
}

func handleInbox(ctx context.Context, request LambdaRequest) (*LambdaResponse, error) {
	HOST_SITE := GetHostSite(ctx)
	fmt.Println("Headers:", request.Headers)
	fmt.Println("Body:", request.Body)

	requestJSON := make(map[string]any)
	err := json.Unmarshal([]byte(request.Body), &requestJSON)
	if err != nil {
		return getLambdaResp(fmt.Errorf(
			"%w: bad json syntax: %s", ErrBadRequest, err.Error()))
	}

	fmt.Println("Request type:", requestJSON["type"])

	switch requestJSON["type"] {
	case "Follow":
		actorObj, err := HandleFollow(&request, requestJSON)
		if err != nil {
			return getLambdaResp(err)
		}

		if err := CallFollowService(&request, HOST_SITE, actorObj); err != nil {
			return getLambdaResp(err)
		}

		return getLambdaResp(nil)

	case "Create":
		err = HandleReply(&request, requestJSON, HOST_SITE)
		if err != nil {
			return getLambdaResp(err)
		}

	case "Undo":
		var err error
		object, ok := requestJSON["object"].(map[string]any)
		if !ok || object["type"] != "Follow" {
			break
		}

		err = HandleUnfollow(&request, requestJSON)
		if err != nil {
			return getLambdaResp(err)
		}

		return getLambdaResp(nil)
	}

	return getLambdaResp(fmt.Errorf(
		"%w: unsupported operation", ErrNotImplemented))
}

func HandleFollow(r *LambdaRequest, reqJSON map[string]any) (*ap.Actor, error) {
	actor, err := ap.RecvActivity(r, reqJSON)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return nil, fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	// write to json database
	actorAt := ap.GetActorAt(actor)
	_, err = client.Collection("followers").Doc(actorAt).Set(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("failed adding follower: %v", err)
	}

	return actor, nil
}

func HandleUnfollow(r *LambdaRequest, requestJSON map[string]any) error {
	actor, err := ap.RecvActivity(r, requestJSON)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	// write to json database
	actorAt := ap.GetActorAt(actor)
	_, err = client.Collection("followers").Doc(actorAt).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove follower: %v", err)
	}

	return nil
}

func HandleReply(r *LambdaRequest, reqJSON map[string]any, host string) error {
	actor, err := ap.RecvActivity(r, reqJSON)
	if err != nil {
		return err
	}
	var c struct {
		Object ap.Reply `json:"object"`
	}
	err = json.Unmarshal([]byte(r.Body), &c)
	if err != nil {
		return err
	}
	replyObj := c.Object

	// validate reply properties
	inReplyTo := replyObj.InReplyTo
	if inReplyTo == "" {
		return fmt.Errorf("%w: inReplyTo not provided", ErrBadRequest)
	}
	fmt.Println(inReplyTo)
	_, err = time.Parse(time.RFC3339, replyObj.Published)
	if err != nil {
		return fmt.Errorf("%w: bad published timestamp: %w", ErrBadRequest, err)
	}
	_, err = url.ParseRequestURI(replyObj.URL)
	if err != nil {
		return fmt.Errorf("%w: malformed backlink URL: %w", ErrBadRequest, err)
	}
	if replyObj.AttributedTo != actor.Id {
		return fmt.Errorf("%w: actor and attributedTo mismatch", ErrBadRequest)
	}

	inReplyToURI, err := url.ParseRequestURI(inReplyTo)
	if err != nil {
		return fmt.Errorf("%w: malformed inReplyTo URI: %w", ErrBadRequest, err)
	}
	inReplyToSlug := Sluggify(*inReplyToURI)
	if replyObj.Id == "" || replyObj.Content == "" {
		return fmt.Errorf("%w: missing reply details", ErrBadRequest)
	}
	replyObjId, err := url.ParseRequestURI(replyObj.Id)
	if err != nil {
		return fmt.Errorf("%w: malformed object id: %w", ErrBadRequest, err)
	}
	replyIdSlug := Sluggify(*replyObjId)

	replyObj.Actor = actor

	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	// check if inReplyTo's object exists in the replies collection
	_, err = client.Collection("replies").Doc(inReplyToSlug).Get(ctx)
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("error looking up replies: %w", err)
	}

	// this post isn't in replies collection yet - confirm post exists
	if inReplyToURI.Host != host {
		return fmt.Errorf("%w: reply not from this domain", ErrBadRequest)
	}
	resp, err := http.Head(inReplyTo)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: referenced post nonexistent", ErrBadRequest)
	}
	fmt.Println("Post", inReplyTo, "found")

	// We need to write two documents: the reply being added, and the original
	// post (which may not exist yet) to link it to the newly created reply.

	repliesCollection := client.Collection("replies")
	txFunc := func(ctx context.Context, tx *firestore.Transaction) error {
		// this will fail if the reply ID already exists
		newReplyDoc := repliesCollection.Doc(replyIdSlug)
		if err := tx.Create(newReplyDoc, replyObj); err != nil {
			return err
		}

		// If it's the first comment to a top-level post, we will create a new
		// reply document for it. Otherwise, we will just merge the reply sets.
		// For replies-to-replies, the parent reply will already exist.
		return tx.Set(repliesCollection.Doc(inReplyToSlug), map[string]any{
			"Id": inReplyTo,
			"Replies": map[string]any{ // will clobber other fields in struct
				"Id":    inReplyToURI.JoinPath("replies").String(),
				"Items": firestore.ArrayUnion(replyObj.Id),
			},
		}, firestore.MergeAll)
	}
	if err = client.RunTransaction(ctx, txFunc); err != nil {
		return err
	}

	return nil
}

// Invokes the serverless function to send an AcceptFollow to the actor's inbox
func CallFollowService(r *LambdaRequest, host string, actor *ap.Actor) error {
	actorBytes, err := json.Marshal(actor)
	if err != nil {
		return fmt.Errorf("%w: could not encode actor string: %w",
			ErrBadRequest, err)
	}
	followReq := ap.FollowServiceRequest{
		FollowObj: r.Body,
		Actor:     actorBytes,
	}
	reqBody, err := json.Marshal(followReq)
	if err != nil {
		return fmt.Errorf("%w: could not encode reply request: %w",
			ErrBadRequest, err)
	}

	// fire and forget
	go func() {
		url := host + "/ap/follow-service"
		req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
		if err != nil {
			fmt.Println("could not form request:", err)
			return
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Authorization", os.Getenv("SELF_API_KEY"))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println("could not send post to follow service:", err)
			return
		}
		fmt.Println("Resp:", resp, "Err:", err)
	}()

	// give the follow service time to read request body
	time.Sleep(50 * time.Millisecond)

	return nil
}

func getLambdaResp(err error) (*LambdaResponse, error) {
	var code int
	if errors.Is(err, ErrUnauthorized) {
		code = http.StatusUnauthorized
	} else if errors.Is(err, ErrBadRequest) {
		code = http.StatusBadRequest
	} else if errors.Is(err, ErrNotImplemented) {
		code = http.StatusNotImplemented
	} else if err != nil {
		code = http.StatusInternalServerError
	} else {
		code = http.StatusOK
	}

	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}
	fmt.Println(code, err)
	return &events.APIGatewayProxyResponse{
		StatusCode: code,
		Body:       errMsg,
	}, nil
}
