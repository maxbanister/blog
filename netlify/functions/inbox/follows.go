package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/maxbanister/blog/util"

	"github.com/maxbanister/blog/ap"
	"github.com/maxbanister/blog/kv"
)

func HandleFollow(actor *ap.Actor, reqJSON map[string]any) error {
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	// write to json database
	actorAt := ap.GetActorAt(actor)
	_, err = client.Collection("followers").Doc(actorAt).Set(ctx, actor)
	if err != nil {
		return fmt.Errorf("failed adding follower: %v", err)
	}

	return nil
}

func HandleUnfollow(actor *ap.Actor, requestJSON map[string]any) error {
	ctx := context.Background()
	client, err := kv.GetFirestoreClient()
	if err != nil {
		return fmt.Errorf("could not start firestore client: %w", err)
	}
	defer client.Close()

	// write to json database
	actorAt := ap.GetActorAt(actor)
	_, err = client.Collection("followers").Doc(actorAt).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove follower: %v", err)
	}

	return nil
}

// Invokes the serverless function to send an AcceptFollow to the actor's inbox
func CallFollowService(r *LambdaRequest, host string, actor *ap.Actor) error {
	actorBytes, err := json.Marshal(actor)
	if err != nil {
		return fmt.Errorf("%w: could not encode actor string: %w",
			ErrBadRequest, err)
	}
	followReq := ap.FollowServiceRequest{
		FollowObj: r.Body,
		Actor:     actorBytes,
	}
	reqBody, err := json.Marshal(followReq)
	if err != nil {
		return fmt.Errorf("%w: could not encode reply request: %w",
			ErrBadRequest, err)
	}

	// fire and forget
	go func() {
		url := "https://" + host + "/ap/follow-service"
		req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
		if err != nil {
			fmt.Println("could not form request:", err)
			return
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Authorization", os.Getenv("SELF_API_KEY"))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println("could not send post to follow service:", err)
			return
		}
		fmt.Println("Resp:", resp, "Err:", err)
	}()

	// give the follow service time to read request body
	time.Sleep(50 * time.Millisecond)

	return nil
}
