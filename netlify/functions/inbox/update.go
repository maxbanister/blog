package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/maxbanister/blog/ap"
	"github.com/maxbanister/blog/kv"
	. "github.com/maxbanister/blog/util"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func HandleProfileUpdate(r *LambdaRequest, reqJSON map[string]any) error {
	object, _ := reqJSON["object"].(map[string]any)
	if reqJSON["actor"] != object["id"] {
		return fmt.Errorf("%w: actor must be equal to object id", ErrBadRequest)
	}
	actor, err := ap.RecvActivity(r, reqJSON)
	if err != nil {
		return err
	}
	actorAt := ap.GetActorAt(actor)
	fmt.Println("Got profile update for", actorAt)

	// check if follower exists, if so update there
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	followersCol := client.Collection("followers")
	ctx := context.Background()
	// can't update with a struct using the firestore SDK
	_, err = followersCol.Doc(actorAt).Update(ctx, []firestore.Update{
		{Path: "Name", Value: actor.Name},
		{Path: "PreferredUsername", Value: actor.PreferredUsername},
		{Path: "Inbox", Value: actor.Inbox},
		{Path: "Icon", Value: actor.Icon},
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			fmt.Println("actor not in followers")
		} else {
			return fmt.Errorf("could not update followers: %w", err)
		}
	}

	// query for all actor's with this ID in replies, update those
	repliesCol := client.Collection("replies")
	bulkWriter := client.BulkWriter(ctx)
	defer bulkWriter.End()
	// empty projection because we only want document refs
	iter := repliesCol.Select().Where("Actor.Id", "==", actor.Id).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("document iterator error: %w", err)
		}
		fmt.Println("Updating reply document ref", doc.Ref.ID)
		_, err = bulkWriter.Update(doc.Ref, []firestore.Update{{
			Path: "Actor",
			Value: ap.Actor{
				Id:                actor.Id,
				Name:              actor.Name,
				PreferredUsername: actor.PreferredUsername,
				Inbox:             actor.Inbox,
				Icon:              actor.Icon,
			}},
		})
		if err != nil {
			return fmt.Errorf("could not update replies: %w", err)
		}
	}

	return nil
}

func HandleReplyEdit(r *LambdaRequest, reqJSON map[string]any) error {
	_, err := ap.RecvActivity(r, reqJSON)
	if err != nil {
		return err
	}
	return nil
}
