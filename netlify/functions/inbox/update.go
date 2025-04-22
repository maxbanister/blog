package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

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
	_, err := ap.RecvActivity(r, reqJSON)
	if err != nil {
		return err
	}

	// We can't use the fetched actor, since it's been observed that it updates
	// after the update activity is sent. So, we copy this info from the object
	var a struct {
		Object ap.Actor `json:"object"`
	}
	err = json.Unmarshal([]byte(r.Body), &a)
	if err != nil {
		return fmt.Errorf("%w: could not decode object: %w", ErrBadRequest, err)
	}
	a.Object.PublicKey = nil
	if actorIcon, ok := object["icon"].(map[string]any); ok {
		a.Object.Icon = actorIcon["url"]
	}
	actor := a.Object
	actorAt := ap.GetActorAt(&actor)
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

func HandleReplyEdit(r *LambdaRequest, reqJSON map[string]any) error {
	_, err := ap.RecvActivity(r, reqJSON)
	if err != nil {
		return err
	}

	editedObj, _ := reqJSON["object"].(map[string]any)
	id, _ := editedObj["id"].(string)
	if id == "" {
		fmt.Println("%w: malformed update object", ErrBadRequest)
	}
	replyURI, err := url.ParseRequestURI(id)
	if err != nil {
		fmt.Println("%w: unable to parse object id URI", ErrBadRequest)
	}
	slugReplyID := Sluggify(*replyURI)
	fmt.Println("Attempting edit of", slugReplyID)

	// fetch note object from firestore
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	docRef := client.Collection("replies").Doc(slugReplyID)
	ctx := context.Background()

	txFunc := func(ctx context.Context, tx *firestore.Transaction) error {
		// get stored object
		doc, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) != codes.NotFound {
				return fmt.Errorf("error accessing replies doc: %w", err)
			}
			return fmt.Errorf("%w: could not find reply ID", ErrBadRequest)
		}
		var storedReply ap.Reply
		err = doc.DataTo(&storedReply)
		if err != nil {
			return fmt.Errorf("couldn't unmarshal stored reply object: %w", err)
		}

		// validate edit object
		editDate, ok := editedObj["updated"].(string)
		if !ok {
			return fmt.Errorf("%w: no \"updated\" time provided", ErrBadRequest)
		}
		if editDate < storedReply.Updated {
			return fmt.Errorf("%w: provided object predates existing object",
				ErrBadRequest)
		}
		editedContent, _ := editedObj["content"].(string)
		if editedContent == "" {
			return fmt.Errorf("%w: must provide update content", ErrBadRequest)
		}
		if len(storedReply.Replies.Items) > 0 {
			var re = regexp.MustCompile(`(</\w+>)*$`)
			old := re.ReplaceAllLiteralString(storedReply.Content, "")
			new := re.ReplaceAllLiteralString(editedContent, "")
			fmt.Println("Old:", old)
			fmt.Println("New:", new)
			if !strings.HasPrefix(new, old) {
				return fmt.Errorf(
					"%w: updates to replied-to notes are append-only",
					ErrBadRequest)
			}
		}

		// update stored object
		err = tx.Update(docRef, []firestore.Update{
			{Path: "Updated", Value: editDate},
			{Path: "URL", Value: editedObj["url"]},
			{Path: "Content", Value: editedContent},
		})
		if err != nil {
			return fmt.Errorf("could not update reply doc: %w", err)
		}

		return nil
	}

	return client.RunTransaction(ctx, txFunc)
}
