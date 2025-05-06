package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/maxbanister/blog/netlify/ap"
	"github.com/maxbanister/blog/netlify/kv"
	. "github.com/maxbanister/blog/netlify/util"
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
	// object in this context is the original post being liked/shared
	objectURIString, _ := reqJSON["object"].(string)
	objectURI, err := url.Parse(objectURIString)
	if err != nil {
		return fmt.Errorf("%w: malformed object URI: %w", ErrBadRequest, err)
	}
	slugObject := Sluggify(*objectURI)

	endorseBackLink, _ := reqJSON["url"].(string)

	// this is the id of the like/share activity
	endorseURIString, _ := reqJSON["id"].(string)
	endorseURI, err := url.Parse(endorseURIString)
	if err != nil {
		return fmt.Errorf("%w: malformed ID URI: %w", ErrBadRequest, err)
	}
	slugEndorse := Sluggify(*endorseURI)

	// open database connection to firestore
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	collectionRef := client.Collection(colName)
	objectDocRef := collectionRef.Doc(slugObject)
	endorseDocRef := collectionRef.Doc(slugEndorse)

	// check if post exists
	fmt.Println("Checking for", objectURIString)
	_, err = objectDocRef.Get(ctx)
	if err != nil {
		if status.Code(err) != codes.NotFound {
			return fmt.Errorf("error looking up document: %w", err)
		}
		// this post isn't in the collection yet - confirm post exists
		_, host, _ := strings.Cut(host, "//")
		if host != objectURI.Host {
			return fmt.Errorf("%w: post not in this domain", ErrBadRequest)
		}
		resp, err := http.Head(objectURIString)
		if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("%w: referenced post nonexistent", ErrBadRequest)
		}
	}
	fmt.Println("Post", objectURIString, "found")

	txFunc := func(ctx context.Context, tx *firestore.Transaction) error {
		// add to object's list of likes/shares
		err = tx.Set(objectDocRef, map[string]any{
			"Id":    objectURI.JoinPath(colName).String(),
			"Items": firestore.ArrayUnion(endorseURIString),
		}, firestore.MergeAll)
		if err != nil {
			return fmt.Errorf("failed to add item: %v", err)
		}

		// create like/share activity
		err = tx.Set(endorseDocRef, map[string]any{
			"Id":     endorseURIString,
			"URL":    endorseBackLink,
			"Object": objectURIString,
			"Actor":  a,
		})
		if err != nil {
			return fmt.Errorf("failed to add item: %v", err)
		}
		return nil
	}

	return client.RunTransaction(ctx, txFunc)
}

func unendorse(reqJSON map[string]any, colName string) error {
	// object in this context is the original post being liked/shared
	object, _ := reqJSON["object"].(map[string]any)

	// this is the id of the undo like/share activity
	objectID, _ := object["id"].(string)
	objectIDURI, err := url.Parse(objectID)
	if err != nil {
		return fmt.Errorf("%w: malformed ID URI: %w", ErrBadRequest, err)
	}
	slugObjID := Sluggify(*objectIDURI)

	// open database connection to firestore
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	collectionRef := client.Collection(colName)
	likeOrShareDocRef := collectionRef.Doc(slugObjID)

	// Need to get the ID of the post this like/share refers to from firestore
	fmt.Printf("Getting %s/%s\n", colName, slugObjID)
	likeOrShareDoc, err := likeOrShareDocRef.Get(ctx)
	if err != nil {
		return fmt.Errorf("error looking up document: %w", err)
	}
	originalPostURI, err := likeOrShareDoc.DataAt("Object")
	originalPostURIStr, _ := originalPostURI.(string)
	if originalPostURIStr == "" || err != nil {
		return fmt.Errorf("error getting document data: %w", err)
	}

	opURI, err := url.Parse(originalPostURIStr)
	if err != nil {
		return fmt.Errorf("%w: malformed object URI: %w", ErrBadRequest, err)
	}
	slugOPURI := Sluggify(*opURI)
	originalPostDocRef := collectionRef.Doc(slugOPURI)

	fmt.Printf("Attempting to remove %s/%s\n", colName, slugObjID)

	txFunc := func(ctx context.Context, tx *firestore.Transaction) error {
		err = tx.Delete(likeOrShareDocRef)
		if err != nil {
			return fmt.Errorf("failed to get item: %w", err)
		}
		err = tx.Update(originalPostDocRef, []firestore.Update{
			{Path: "Items", Value: firestore.ArrayRemove(objectID)},
		})
		if err != nil {
			return fmt.Errorf("failed to remove item: %w", err)
		}
		return nil
	}

	return client.RunTransaction(ctx, txFunc)
}
