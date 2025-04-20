package main

import (
	"context"
	"fmt"
	"net/url"

	"cloud.google.com/go/firestore"
	"github.com/maxbanister/blog/ap"
	"github.com/maxbanister/blog/kv"
	. "github.com/maxbanister/blog/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func HandleDelete(r *LambdaRequest, reqJSON map[string]any) error {
	replyID, _ := reqJSON["id"].(string)
	if replyID == "" {
		return fmt.Errorf("%w: no ID string in request", ErrBadRequest)
	}
	replyURI, err := url.Parse(replyID)
	if err != nil {
		return fmt.Errorf("%w: couldn't parse ID as URI: %w", ErrBadRequest, err)
	}
	slugReplyID := Sluggify(*replyURI)

	// lookup object id in replies
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	fmt.Println("Deleting", slugReplyID)
	repliesCol := client.Collection("replies")
	doc, err := repliesCol.Doc(slugReplyID).Get(ctx)
	if err != nil {
		if status.Code(err) != codes.NotFound {
			return fmt.Errorf("error looking up replies: %w", err)
		}
		return fmt.Errorf("reply document nonexistent: %w", err)
	}
	var replyObj ap.Reply
	err = doc.DataTo(&replyObj)
	if err != nil {
		return fmt.Errorf("could not convert document to struct: %w", err)
	}

	// if this item is in the middle of a reply chain, just make it a tombstone
	if len(replyObj.Replies.Items) > 0 {
		_, err = repliesCol.Doc(slugReplyID).Update(ctx, []firestore.Update{
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
		fmt.Println("Successfully entombed reply node", slugReplyID)
		return nil
	}

	// If it's a leaf (reply items is empty), delete this collection.
	// Traverse up the chain using InReplyTo to find tombstones, and remove them
	// until coming across one that has more than zero replyItems
	for {
		_, err = repliesCol.Doc(slugReplyID).Delete(ctx)
		if err != nil {
			return fmt.Errorf("failed to remove leaf reply: %v", err)
		}
		fmt.Println("Successful delete of leaf node", slugReplyID)
		replyURI, err = url.Parse(replyObj.InReplyTo)
		if err != nil {
			return err
		}
		slugReplyID = Sluggify(*replyURI)
		doc, err = repliesCol.Doc(slugReplyID).Get(ctx)
		if err != nil {
			if status.Code(err) != codes.NotFound {
				return fmt.Errorf("error looking up replies: %w", err)
			}
			return fmt.Errorf("InReplyTo reference broken: %s", slugReplyID)
		}
		err = doc.DataTo(&replyObj)
		if err != nil {
			return fmt.Errorf("could not convert document to struct: %w", err)
		}
		if replyObj.Type != "Tombstone" || len(replyObj.Replies.Items) > 0 {
			break
		}
	}

	return nil
}
