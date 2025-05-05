package main

import (
	"context"
	"fmt"
	"net/url"

	"cloud.google.com/go/firestore"
	"github.com/maxbanister/blog/netlify/ap"
	"github.com/maxbanister/blog/netlify/kv"
	. "github.com/maxbanister/blog/netlify/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func HandleDelete(reqJSON map[string]any) error {
	nestedObj, _ := reqJSON["object"].(map[string]any)
	deleteID, _ := nestedObj["id"].(string)
	if deleteID == "" {
		return fmt.Errorf("%w: no ID string in request", ErrBadRequest)
	}
	replyURI, err := url.Parse(deleteID)
	if err != nil {
		return fmt.Errorf("%w: couldn't parse ID as URI: %w", ErrBadRequest, err)
	}
	slugDeleteID := Sluggify(*replyURI)

	// lookup object id in replies
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	fmt.Println("Attempting to delete", slugDeleteID)
	repliesCol := client.Collection("replies")
	doc, err := repliesCol.Doc(slugDeleteID).Get(ctx)
	if err != nil {
		if status.Code(err) != codes.NotFound {
			return fmt.Errorf("error looking up replies: %w", err)
		}
		// Mastodon sometimes resends deletes; a 2XX response code makes it stop
		return fmt.Errorf("%w: reply document nonexistent", ErrAlreadyDone)
	}
	var deleteObj ap.Reply
	err = doc.DataTo(&deleteObj)
	if err != nil {
		return fmt.Errorf("could not convert document to struct: %w", err)
	}

	// if this item is in the middle of a reply chain, just make it a tombstone
	if len(deleteObj.Replies.Items) > 0 {
		_, err = repliesCol.Doc(slugDeleteID).Update(ctx, []firestore.Update{
			{Path: "Type", Value: "Tombstone"},
			{Path: "URL", Value: ""},
			{Path: "AttributedTo", Value: ""},
			{Path: "To", Value: nil},
			{Path: "Cc", Value: nil},
			{Path: "Content", Value: ""},
			{Path: "Actor", Value: nil},
		})
		if err != nil {
			return fmt.Errorf("failed to remove leaf reply: %v", err)
		}
		fmt.Println("Successfully entombed reply node", slugDeleteID)
		return nil
	}

	// If it's a leaf (reply items is empty), delete this document.
	// Traverse up the chain using InReplyTo to find tombstones, and remove them
	// until coming across one that has more than zero replyItems
	for {
		_, err := repliesCol.Doc(slugDeleteID).Delete(ctx)
		if err != nil {
			return fmt.Errorf("failed to remove leaf reply: %w", err)
		}
		fmt.Println("Successful delete of leaf node", slugDeleteID)
		replyURI, err = url.Parse(deleteObj.InReplyTo)
		if err != nil {
			return err
		}
		slugDeleteID = Sluggify(*replyURI)
		_, err = repliesCol.Doc(slugDeleteID).Update(ctx, []firestore.Update{
			{Path: "Replies.Items", Value: firestore.ArrayRemove(deleteObj.Id)},
		})
		if err != nil {
			if status.Code(err) != codes.NotFound {
				return fmt.Errorf("error accessing replies doc: %w", err)
			}
			return fmt.Errorf("InReplyTo reference broken: %s", slugDeleteID)
		}
		fmt.Println("Successfuly delinked ID from", slugDeleteID)

		doc, err := repliesCol.Doc(slugDeleteID).Get(ctx)
		if err != nil {
			if status.Code(err) != codes.NotFound {
				return fmt.Errorf("error accessing replies doc: %w", err)
			}
			return err
		}
		deleteObj = ap.Reply{}
		err = doc.DataTo(&deleteObj)
		if err != nil {
			return fmt.Errorf("could not convert document to struct: %w", err)
		}
		if deleteObj.Type != "Tombstone" || len(deleteObj.Replies.Items) > 0 {
			break
		}
	}

	return nil
}
