package main

import (
	"embed"
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
	fs2 := handleLog(fs)

	// Serve static files
	http.HandleFunc("/ap/users/@blog", redirectUser)
	http.HandleFunc("/ap/outbox", handleJSON)
	http.HandleFunc("/posts/", handleCondJSON)
	http.HandleFunc("/.well-known/webfinger", handleJSON)
	http.Handle("/", fs2)

	log.Fatal(http.ListenAndServe(":9000", nil))
}

func handleLog(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.URL.Path)
		log.Println(r.Header)
		body, err := r.GetBody()
		if err != nil {
			log.Println(err)
		} else {
			log.Println(body)
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
