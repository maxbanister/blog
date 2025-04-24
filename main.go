package main

import (
	"embed"
	"fmt"
	"io"
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

	// Serve static files
	http.HandleFunc("/@blog", redirectBlog)
	http.HandleFunc("/outbox", outbox)
	http.HandleFunc("/.well-known/webfinger", webfinger)
	http.Handle("/", printRequest(fs))

	err = http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func redirectBlog(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	body, _ := io.ReadAll(r.Body)
	log.Println(string(body))
	log.Println(r.Header)

	fmt.Println(r.Header.Values("Accept"))
	if acceptValues(r.Header.Values("Accept")) {
		fmt.Println("here")
		w.Header().Set("Content-Type", "application/activity+json")
		http.ServeFile(w, r, "public/@blog")
	} else {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func webfinger(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	body, _ := io.ReadAll(r.Body)
	log.Println(string(body))
	w.Header().Set("Content-Type", "application/activity+json")
	http.ServeFile(w, r, "public/.well-known/webfinger")
}

func outbox(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	body, _ := io.ReadAll(r.Body)
	log.Println(string(body))
	w.Header().Set("Content-Type", "application/activity+json")
	http.ServeFile(w, r, "public/outbox")
}

func printRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.URL)
		body, _ := io.ReadAll(r.Body)
		log.Println(string(body))
		h.ServeHTTP(w, r)
	})
}

func acceptValues(vals []string) bool {
	for _, val := range vals {
		if strings.Contains(val, "application/ld+json") ||
			strings.Contains(val, "application/activity+json") ||
			strings.Contains(val, "application/json") {
			return true
		}
	}
	return false
}
