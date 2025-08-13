package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/maxbanister/blog/netlify/ap"
)

func main() {
	userURL := os.Args[1]
	fmt.Println("Attempting to follow", userURL)

	priv_key_contents, err := os.ReadFile("../../private.pem")
	if err != nil {
		fmt.Println("could not find and read private.pem", err.Error())
		return
	}

	os.Setenv("AP_PRIVATE_KEY", string(priv_key_contents))

	actor, err := ap.FetchActorAuthorized(userURL)
	if err != nil {
		fmt.Println("could not fetch actor:", err.Error())
		return
	}
	return

	payload := fmt.Sprintf(`{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id": "https://maxbanister.com/follows/%s",
		"type": "Follow",
		"actor": "https://maxbanister.com/ap/user/max",
		"object": "%s"%s`, randomBase16String(), userURL, "\n}\n")

	err = ap.SendActivity(payload, actor)
	if err != nil {
		fmt.Println("error sending activity:", err.Error())
		return
	}

	os.Unsetenv("AP_PRIVATE_KEY")

	fmt.Println("Successfully sent follow - check inbox for AcceptFollow")
}

func randomBase16String() string {
	buff := make([]byte, 16)
	rand.Read(buff)
	return hex.EncodeToString(buff)
}
