package kv

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/maxbanister/blog/netlify/ap"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func UpdateAllActorRefs(actor *ap.Actor) error {
	actorAt := ap.GetActorAt(actor)
	fmt.Println("Got profile update for", actorAt)

	// check if follower exists, if so update there
	client, err := GetFirestoreClient()
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
	} else {
		fmt.Println("Sucessfully updated actor in followers")
	}

	bulkWriter := client.BulkWriter(ctx)
	defer bulkWriter.End()

	// query for all actors with this ID in these collections and update those
	for _, colName := range []string{"replies", "likes", "shares"} {
		col := client.Collection(colName)
		// empty projection because we only need document refs
		iter := col.Select().Where("Actor.Id", "==", actor.Id).Documents(ctx)
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("document iterator error: %w", err)
			}
			fmt.Printf("Updating document ref %s/%s\n", colName, doc.Ref.ID)
			_, err = bulkWriter.Update(doc.Ref, []firestore.Update{
				{Path: "Actor", Value: &actor},
			})
			if err != nil {
				return fmt.Errorf("could not update collection: %w", err)
			}
		}
		bulkWriter.Flush()
	}

	return nil
}
