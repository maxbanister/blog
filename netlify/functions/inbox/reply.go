package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/maxbanister/blog/ap"
	"github.com/maxbanister/blog/kv"
	. "github.com/maxbanister/blog/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func HandleReply(r *LambdaRequest, actor *ap.Actor, reqJSON map[string]any, host string) error {
	var c struct {
		Object ap.Reply `json:"object"`
	}
	err := json.Unmarshal([]byte(r.Body), &c)
	if err != nil {
		return err
	}
	replyObj := c.Object

	// validate reply properties
	inReplyTo := replyObj.InReplyTo
	if inReplyTo == "" {
		return fmt.Errorf("%w: inReplyTo not provided", ErrBadRequest)
	}
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

	fmt.Println("Checking for", inReplyTo)
	repliesCollection := client.Collection("replies")
	// check if inReplyTo's object exists in the replies collection
	_, err = repliesCollection.Doc(inReplyToSlug).Get(ctx)
	if err != nil {
		if status.Code(err) != codes.NotFound {
			return fmt.Errorf("error looking up replies: %w", err)
		}
		// this post isn't in the replies collection yet - confirm post exists
		if inReplyToURI.Host != host {
			return fmt.Errorf("%w: reply not for this domain", ErrBadRequest)
		}
		resp, err := http.Head(inReplyTo)
		if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("%w: referenced post nonexistent", ErrBadRequest)
		}
	}
	fmt.Println("Post", inReplyTo, "found")

	// We need to write two documents: the reply being added, and the original
	// post (which may not exist yet) to link it to the newly created reply.

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
