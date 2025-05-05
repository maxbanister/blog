package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/maxbanister/blog/netlify/ap"
)

func main() {
	userURL := os.Args[1]
	fmt.Println("Sending message to", userURL)

	req, err := http.NewRequest("GET", userURL, bytes.NewBuffer([]byte{}))
	if err != nil {
		fmt.Println("could not create request:", err.Error())
		return
	}
	req.Header.Set("Accept", "application/activity+json")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		fmt.Println("http request error:", err.Error())
		return
	}

	var actor ap.Actor
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&actor)
	if err != nil {
		fmt.Println("error unmarshalling actor:", err.Error())
		return
	}

	randVal := randomBase16String()
	payload := fmt.Sprintf(`{
		"@context": [
		  "https://www.w3.org/ns/activitystreams"
		],
		"id": "https://maxbanister.com/posts/%s#create",
		"type": "Create",
		"actor": "https://maxbanister.com/ap/user/max",
		"published": "2025-05-03T22:46:52Z",
		"to": [
		  "%s"
		],
		"cc": [],
		"object": {
		  "@context": "https://www.w3.org/ns/activitystreams",
		  "id": "https://maxbanister.com/posts/%s",
		  "type": "Note",
		  "summary": null,
		  "inReplyTo": null,
		  "published": "2025-05-05T07:46:52Z",
		  "url": "https://maxbanister.com/posts/%s",
		  "attributedTo": "https://maxbanister.com/ap/user/max",
		  "to": [
			"%s"
		  ],
		  "cc": [],
		  "content": "username maxbanister.com"
		}
	  }`, randVal, userURL, randVal, randVal, userURL)

	priv_key_contents, err := os.ReadFile("../../private.pem")
	if err != nil {
		fmt.Println("could not find and read private.pem", err.Error())
		return
	}

	os.Setenv("AP_PRIVATE_KEY", string(priv_key_contents))

	err = ap.SendActivity(payload, &actor)
	if err != nil {
		fmt.Println("error sending activity:", err.Error())
		return
	}

	os.Unsetenv("AP_PRIVATE_KEY")

	fmt.Println("Successfully sent message")
}

func randomBase16String() string {
	buff := make([]byte, 16)
	rand.Read(buff)
	return hex.EncodeToString(buff)
}
