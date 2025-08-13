package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"cloud.google.com/go/firestore"
	"github.com/maxbanister/blog/netlify/ap"
	"github.com/maxbanister/blog/netlify/kv"
	. "github.com/maxbanister/blog/netlify/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func HandleProfileUpdate(r *LambdaRequest, reqJSON map[string]any) error {
	object, _ := reqJSON["object"].(map[string]any)
	if reqJSON["actor"] != object["id"] {
		return fmt.Errorf("%w: actor must be equal to object id", ErrBadRequest)
	}

	// We can't use the fetched actor, since it's been observed that it updates
	// after the update activity is sent. So, we copy this info from the object
	var a struct {
		Object ap.Actor `json:"object"`
	}
	err := json.Unmarshal([]byte(r.Body), &a)
	if err != nil {
		return fmt.Errorf("%w: could not decode object: %w", ErrBadRequest, err)
	}
	a.Object.PublicKey = nil
	if actorIcon, ok := object["icon"].(map[string]any); ok {
		a.Object.Icon = actorIcon["url"]
	}
	actor := a.Object

	return kv.UpdateAllActorRefs(&actor)
}

func HandleReplyEdit(r *LambdaRequest, reqJSON map[string]any) error {
	editedObj, _ := reqJSON["object"].(map[string]any)
	id, _ := editedObj["id"].(string)
	if id == "" {
		fmt.Println("%w: malformed update object", ErrBadRequest)
	}
	replyURI, err := url.Parse(id)
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
