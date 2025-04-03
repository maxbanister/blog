package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed public/*
var staticFiles embed.FS

func main() {
	var staticFS = fs.FS(staticFiles)
	htmlContent, err := fs.Sub(staticFS, "public")
	if err != nil {
		log.Fatal(err)
	}
	fs := http.FileServer(http.FS(htmlContent))

	mux := http.NewServeMux()

	// Serve static files
	mux.HandleFunc("/ap/users/@blog", redirectUser)
	mux.HandleFunc("/ap/outbox", handleJSON)
	mux.HandleFunc("/ap/inbox", handleInbox)
	mux.HandleFunc("/posts/", handleCondJSON)
	mux.HandleFunc("/.well-known/webfinger", handleJSON)
	mux.Handle("/", fs)

	log.Fatal(http.ListenAndServe(":9000", logHandler(mux)))
}

func logHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("Path:", r.URL.Path)
		log.Println("Headers:", r.Header)
		buf, _ := io.ReadAll(r.Body)
		newBody := io.NopCloser(bytes.NewBuffer(buf))
		r.Body = newBody
		if len(buf) > 0 {
			log.Println("Body:", string(buf))
		}
		h.ServeHTTP(w, r)
	})
}

func redirectUser(w http.ResponseWriter, r *http.Request) {
	if acceptsJSON(r.Header.Values("Accept")) {
		handleJSON(w, r)
	} else {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func handleCondJSON(w http.ResponseWriter, r *http.Request) {
	if acceptsJSON(r.Header.Values("Accept")) {
		handleJSON(w, r)
	} else {
		http.ServeFile(w, r, filepath.Join("public", r.URL.Path))
	}
}

func handleJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/activity+json")
	http.ServeFile(w, r, filepath.Join("public", r.URL.Path))
}

func acceptsJSON(vals []string) bool {
	opts := []string{"ld+json", "activity+json", "json"}
	for _, val := range vals {
		for _, opt := range opts {
			if strings.Contains(strings.ToLower(val), "application/"+opt) {
				return true
			}
		}
	}
	return false
}

func handleInbox(w http.ResponseWriter, r *http.Request) {
	jsonData := new(map[string]interface{})
	err := json.NewDecoder(r.Body).Decode(&jsonData)
	if err != nil {
		http.Error(w, "bad json syntax: "+err.Error(), http.StatusBadRequest)
	}
	// fetch user object
	// verify signature
	// nothing will send a 200 OK
	go AcceptRequest()
}

func AcceptRequest() {

}
