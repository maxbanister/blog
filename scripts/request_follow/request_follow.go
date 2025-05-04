package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/maxbanister/blog/netlify/ap"
)

func main() {
	userURL := os.Args[1]
	fmt.Println("Attempting to follow", userURL)

	req, err := http.NewRequest("GET", userURL, bytes.NewBuffer([]byte{}))
	if err != nil {
		fmt.Println("could not create request:", err.Error())
		return
	}
	req.Header.Set("Accept", "application/ld+json;")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("http request error:", err.Error())
		return
	}

	stuff, _ := io.ReadAll(resp.Body)
	fmt.Println(string(stuff))

	var actor ap.Actor
	decoder := json.NewDecoder(bytes.NewBuffer(stuff))
	err = decoder.Decode(&actor)
	if err != nil {
		fmt.Println("error unmarshalling actor:", err.Error())
		return
	}

	payload := fmt.Sprintf(`{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id": "https://maxbanister.com/follows/%s",
		"type": "Follow",
		"actor": "https://maxbanister.com/ap/user/max",
		"object": "%s"%s`, randomBase16String(), userURL, "\n}\n")

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

	fmt.Println("Successfully sent follow - check inbox for AcceptFollow")
}

func randomBase16String() string {
	buff := make([]byte, 16)
	rand.Read(buff)
	return hex.EncodeToString(buff)
}
