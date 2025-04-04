package main

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"reflect"
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
	mux.HandleFunc("POST /ap/inbox", handleInbox)
	mux.HandleFunc("/posts/", handleCondJSON)
	mux.HandleFunc("/.well-known/webfinger", handleJSON)
	mux.Handle("/", fs)

	log.Fatal(http.ListenAndServe(":9000", logHandler(mux)))
}

type HTTPLogWriter struct {
	Writer     http.ResponseWriter
	StatusCode int
	ErrMsg     string
}

func NewHTTPLogWriter(w http.ResponseWriter) *HTTPLogWriter {
	return &HTTPLogWriter{w, 0, ""}
}
func (w HTTPLogWriter) Header() http.Header {
	return w.Writer.Header()
}
func (w *HTTPLogWriter) Write(resp []byte) (int, error) {
	if w.StatusCode == 0 {
		w.StatusCode = 200
	} else {
		w.ErrMsg = string(resp)
	}
	return w.Writer.Write(resp)
}
func (w *HTTPLogWriter) WriteHeader(statusCode int) {
	w.StatusCode = statusCode
	w.Writer.WriteHeader(statusCode)
}

func logHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			h.ServeHTTP(w, r)
			return
		}
		log.Println("Path:", r.URL.Path)
		log.Println("Headers:", r.Header)
		buf, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(buf))
		if len(buf) > 0 {
			log.Println("Body:", string(buf))
		}
		lw := NewHTTPLogWriter(w)
		h.ServeHTTP(lw, r)
		code := lw.StatusCode
		if code == 200 {
			log.Println("Response: 200 OK")
		} else if code == 404 || lw.ErrMsg == "" {
			log.Printf("Response: %d %s\n", code, http.StatusText(code))
		} else {
			log.Printf("Response: %d %s - %s\n", code, http.StatusText(code),
				lw.ErrMsg)
		}
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
	digests, ok := r.Header["Digest"]
	if !ok || len(digests) == 0 {
		http.Error(w, "no digest header", http.StatusBadRequest)
		return
	}
	var digestFound bool
	var digestBase64 string
	for _, digest := range digests {
		digestAlgo, digestRaw, hasSep := strings.Cut(digest, "=")
		if !hasSep {
			http.Error(w, "malformed digest header", http.StatusBadRequest)
			return
		}
		if strings.ToLower(digestAlgo) == "sha-256" {
			digestBase64 = digestRaw
			digestFound = true
			break
		}
	}
	if !digestFound {
		http.Error(w, "unsupported digest algorithm", http.StatusBadRequest)
		return
	}
	digestBytes, err := base64.StdEncoding.DecodeString(digestBase64)
	if err != nil {
		http.Error(w, "couldn't decode base64 digest: "+err.Error(),
			http.StatusBadRequest)
		return
	}
	followReqBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "couldn't read request body: "+err.Error(),
			http.StatusBadRequest)
		return
	}
	reqBodyHash := sha256.Sum256(followReqBody)
	// I don't think this needs to be constant time...
	if !bytes.Equal(reqBodyHash[:], digestBytes) {
		http.Error(w, "digest didn't match message body", http.StatusBadRequest)
		return
	}

	signatureHeader, ok := r.Header["Signature"]
	if !ok || len(signatureHeader) == 0 {
		http.Error(w, "no signature header", http.StatusBadRequest)
		return
	}
	var keyID, sigBase64 string
	for _, sig := range strings.Split(signatureHeader[0], ",") {
		sigKey, sigVal, found := strings.Cut(sig, "=")
		if !found || len(sigVal) < 2 {
			continue
		}
		// remove quotes
		sigVal = sigVal[1 : len(sigVal)-1]
		switch sigKey {
		case "signature":
			sigBase64 = sigVal
		case "keyId":
			keyID = sigVal
		case "algorithm":
			if strings.ToLower(sigVal) != "rsa-sha256" {
				http.Error(w, "unsupported signature algorithm",
					http.StatusBadRequest)
				return
			}
		case "headers":
			// headers are always lowercase in signature
			if sigVal != "host date digest content-type (request-target)" {
				http.Error(w, "wrong header order", http.StatusBadRequest)
			}
		}
	}
	if keyID == "" || sigBase64 == "" {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}
	sigBytes, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		http.Error(w, "couldn't decode base64 signature: "+err.Error(),
			http.StatusBadRequest)
		return
	}

	requestJson := new(map[string]interface{})
	err = json.Unmarshal(followReqBody, requestJson)
	if err != nil {
		http.Error(w, "bad json syntax: "+err.Error(), http.StatusBadRequest)
		return
	}
	// fetch user object
	if (*requestJson)["type"] != "Follow" {
		http.Error(w, "unsupported operation", http.StatusNotImplemented)
		return
	}
	actor, ok1 := (*requestJson)["actor"]
	actorURL, ok2 := actor.(string)
	if !ok1 || !ok2 {
		http.Error(w, "no actor found", http.StatusBadRequest)
		return
	}
	keyURL, _, _ := strings.Cut(keyID, "#")
	log.Println("key url:", keyURL)
	fmt.Println("actor url:", actorURL)
	if keyURL != actorURL {
		http.Error(w, "actor does not match key in signature",
			http.StatusBadRequest)
	}

	req, err := http.NewRequest("GET", actorURL, nil)
	req.Header.Set("Accept", `application/ld+json; profile="https://www.w3.org/ns/activitystreams`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	actorJson := new(map[string]interface{})
	err = json.NewDecoder(resp.Body).Decode(&actorJson)
	if err != nil {
		http.Error(w, "bad json syntax: "+err.Error(), http.StatusBadRequest)
		return
	}
	publicKeyJSON, ok1 := (*actorJson)["publicKey"]
	publicKeyJSONMap, ok2 := publicKeyJSON.(map[string]interface{})
	publicKeyPEM, ok3 := publicKeyJSONMap["publicKeyPem"]
	publicKeyPEMStr, ok4 := publicKeyPEM.(string)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		http.Error(w, "no actor public key found", http.StatusBadRequest)
		return
	}
	publicBlock, rest := pem.Decode([]byte(publicKeyPEMStr))
	if rest != nil {
		fmt.Println("rest", rest)
	}
	if publicBlock == nil || publicBlock.Type != "PUBLIC KEY" {
		http.Error(w, "failed to decode public key", http.StatusBadRequest)
		return
	}
	publicKey, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
	if err != nil {
		http.Error(w, "couldn't parse cert: "+err.Error(),
			http.StatusBadRequest)
		return
	}
	rsaPublicKey, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		errMsg := fmt.Sprintf("invalid key type: %s", reflect.TypeOf(publicKey))
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	signingString := "host: " + r.Host + "\n" +
		"date: " + r.Header["Date"][0] + "\n" +
		"digest: " + r.Header["Digest"][0] + "\n" +
		"content-type: " + r.Header["Content-Type"][0] + "\n" +
		"(request-target): post " + r.URL.Path

	hashed := sha256.Sum256([]byte(signingString))
	log.Println("signing string:", signingString)
	log.Println("hashed:", hashed)

	err = rsa.VerifyPKCS1v15(rsaPublicKey, crypto.SHA256, hashed[:], sigBytes)
	if err != nil {
		http.Error(w, "signature did not match digest"+err.Error(),
			http.StatusUnauthorized)
		return
	}

	// verify signature:
	// get http body
	// hash http body
	// check that the hash matches the digest
	// decode pem into byte slice
	// decode the signature using the byte array
	// check that it makes the hash

	// nothing will send a 200 OK
	go AcceptRequest()
}

func AcceptRequest() {

}
