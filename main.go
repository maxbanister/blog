package main

import (
	"embed"
	"io"
	"io/fs"
	"log"
	"net/http"
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
	http.Handle("/", printRequest(fs))

	err = http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func printRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.URL)
		body, _ := io.ReadAll(r.Body)
		log.Println(string(body))
		h.ServeHTTP(w, r)
	})
}
