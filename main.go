package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
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
	http.HandleFunc("/ap/outbox", activityJson)
	http.HandleFunc("/notes/", activityJson)
	http.HandleFunc("/.well-known/webfinger", activityJson)
	http.Handle("/", fs2)

	log.Fatal(http.ListenAndServe(":9000", nil))
}

func handleLog(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.URL.Path)
		h.ServeHTTP(w, r)
	})
}

func redirectUser(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.Header.Values("Accept"))
	if acceptVals(r.Header.Values("Accept")) {
		w.Header().Set("Content-Type", "application/activity+json")
		http.ServeFile(w, r, "public/ap/users/@blog")
	} else {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func activityJson(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/activity+json")
	res := "public" + r.URL.Path
	http.ServeFile(w, r, res)
}

func acceptVals(vals []string) bool {
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
