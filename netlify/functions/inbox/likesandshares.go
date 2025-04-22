package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/maxbanister/blog/ap"
	"github.com/maxbanister/blog/kv"
	. "github.com/maxbanister/blog/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func HandleLike(actor *ap.Actor, reqJSON map[string]any, host string) error {
	return interact(actor, reqJSON, "likes", host)
}

func HandleUnlike(reqJSON map[string]any) error {
	return deinteract(reqJSON, "likes")
}

func HandleAnnounce(actor *ap.Actor, reqJSON map[string]any, host string) error {
	return interact(actor, reqJSON, "shares", host)
}

func HandleUnannounce(reqJSON map[string]any) error {
	return deinteract(reqJSON, "shares")
}

func interact(a *ap.Actor, reqJSON map[string]any, colID, host string) error {
	objectURIString, ok := reqJSON["object"].(string)
	if !ok {
		return fmt.Errorf("%w: object must be URI strin", ErrBadRequest)
	}
	objectURI, err := url.ParseRequestURI(objectURIString)
	if err != nil {
		return fmt.Errorf("%w: malformed object URI: %w", ErrBadRequest, err)
	}
	slugObjURI := Sluggify(*objectURI)

	// open database connection to firestore
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	// check if post exists
	fmt.Println("Checking for", objectURIString)
	docRef := client.Collection(colID).Doc(slugObjURI)
	_, err = docRef.Get(ctx)
	if err != nil {
		if status.Code(err) != codes.NotFound {
			return fmt.Errorf("error looking up document: %w", err)
		}
		// this post isn't in the collection yet - confirm post exists
		if objectURI.Host != host {
			return fmt.Errorf("%w: post not in this domain", ErrBadRequest)
		}
		resp, err := http.Head(objectURIString)
		if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("%w: referenced post nonexistent", ErrBadRequest)
		}
	}
	fmt.Println("Post", objectURIString, "found")

	// set like or share object
	_, err = docRef.Set(ctx, ap.LikeOrShare{
		Object: objectURIString,
		Actor:  a,
	})
	if err != nil {
		return fmt.Errorf("failed to add: %v", err)
	}

	return nil
}

func deinteract(reqJSON map[string]any, colID string) error {
	objectURIString, ok := reqJSON["object"].(string)
	if !ok {
		return fmt.Errorf("%w: object must be URI strin", ErrBadRequest)
	}
	objectURI, err := url.ParseRequestURI(objectURIString)
	if err != nil {
		return fmt.Errorf("%w: malformed object URI: %w", ErrBadRequest, err)
	}
	slugObjURI := Sluggify(*objectURI)

	// open database connection
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	collection := client.Collection(colID)
	_, err = collection.Doc(slugObjURI).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove item: %w", err)
	}

	return nil
}
