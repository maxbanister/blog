package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"cloud.google.com/go/firestore"
	"github.com/maxbanister/blog/ap"
	"github.com/maxbanister/blog/kv"
	. "github.com/maxbanister/blog/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func HandleLike(actor *ap.Actor, reqJSON map[string]any, host string) error {
	return endorse(actor, reqJSON, "likes", host)
}

func HandleUnlike(reqJSON map[string]any) error {
	return unendorse(reqJSON, "likes")
}

func HandleAnnounce(actor *ap.Actor, reqJSON map[string]any, host string) error {
	return endorse(actor, reqJSON, "shares", host)
}

func HandleUnannounce(reqJSON map[string]any) error {
	return unendorse(reqJSON, "shares")
}

func endorse(a *ap.Actor, reqJSON map[string]any, colName, host string) error {
	// object in this context is the post being liked/shared
	objectURIString, ok := reqJSON["object"].(string)
	if !ok {
		return fmt.Errorf("%w: object must be URI strin", ErrBadRequest)
	}
	objectURI, err := url.ParseRequestURI(objectURIString)
	if err != nil {
		return fmt.Errorf("%w: malformed object URI: %w", ErrBadRequest, err)
	}
	slugObjURI := Sluggify(*objectURI)
	endorseID, _ := reqJSON["id"].(string)

	// open database connection to firestore
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	// check if post exists
	fmt.Println("Checking for", objectURIString)
	docRef := client.Collection(colName).Doc(slugObjURI)
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
	_, err = docRef.Set(ctx, map[string]any{
		"Items": firestore.ArrayUnion(&ap.LikeOrShare{
			Id:     endorseID,
			Object: objectURIString,
			Actor:  a,
		}),
	}, firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("failed to add item: %v", err)
	}

	return nil
}

func unendorse(reqJSON map[string]any, colName string) error {
	object, _ := reqJSON["object"].(map[string]any)
	objectID, _ := object["id"].(string)
	if objectID == "" {
		return fmt.Errorf("%w: no id property on object", ErrBadRequest)
	}
	objectURI, err := url.ParseRequestURI(objectID)
	if err != nil {
		return fmt.Errorf("%w: malformed object URI: %w", ErrBadRequest, err)
	}
	slugObjURI := Sluggify(*objectURI)
	fmt.Printf("Attempting to remove %s/%s\n", colName, slugObjURI)

	// open database connection
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	docRef := client.Collection(colName).Doc(slugObjURI)

	// ArrayRemove in Firestore only works on the exact value to be removed, so
	// we must Get the original item to pass to Update
	doc, err := docRef.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}
	var e ap.LikeOrShare
	err = doc.DataTo(&e)
	if err != nil {
		return fmt.Errorf("couldn't unmarshal like or share to struct: %w", err)
	}

	_, err = docRef.Update(ctx, []firestore.Update{
		{Path: "Items", Value: firestore.ArrayRemove(e)},
	})
	if err != nil {
		return fmt.Errorf("failed to remove item: %w", err)
	}

	return nil
}
