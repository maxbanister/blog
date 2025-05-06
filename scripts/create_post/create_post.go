package main

import (
	"bytes"
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

	payload := `{
			"@context": "https://www.w3.org/ns/activitystreams",
			"id": "https://maxbanister.com/posts/post-3/#create",
			"type": "Create",
			"actor": "https://maxbanister.com/ap/user/max",
			"to": [
				"https://www.w3.org/ns/activitystreams#Public"
			],
			"cc": [
				"https://maxbanister.com/ap/followers"
			],
			"published": "2023-03-15T11:00:00-07:00",
			"object": {
				"@context": "https://www.w3.org/ns/activitystreams",
				"id": "https://maxbanister.com/posts/post-3/",
				"type": "Note",
				"content": "Post 3\nOccaecat aliqua consequat laborum ut ex aute aliqua culpa quis irure esse magna dolore quis. Proident fugiat labore eu laboris officia Lorem enim. Ipsum occaecat cillum ut tempor id sint aliqua incididunt nisi incididunt reprehenderit. Voluptate ad minim … https://maxbanister.com/posts/post-3/",
				"url": "https://maxbanister.com/posts/post-3/",
				"attributedTo": "https://maxbanister.com/ap/user/max",
				"to": [
					"https://www.w3.org/ns/activitystreams#Public"
				],
				"cc": [],
				"published": "2023-03-15T11:00:00-07:00",
				"replies": "https://maxbanister.com/posts/post-3/replies",
				"likes": "https://maxbanister.com/posts/post-3/likes",
				"shares": "https://maxbanister.com/posts/post-3/shares",
				"tag": [
					{
						"Type": "Hashtag",
						"Href": "https://maxbanister.com/tags/red",
						"Name": "#red"
					}, 
					{
						"Type": "Hashtag",
						"Href": "https://maxbanister.com/tags/green",
						"Name": "#green"
					}, 
					{
						"Type": "Hashtag",
						"Href": "https://maxbanister.com/tags/blue",
						"Name": "#blue"
					}
				]
			}
		}`

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
