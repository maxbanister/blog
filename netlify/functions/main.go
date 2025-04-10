package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"
)

const SigStringHeaders = "host date digest content-type (request-target)"

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
	reqDate, err := time.Parse(http.TimeFormat, r.Header.Get("Date"))
	if err != nil || time.Since(reqDate) >= 2*time.Hour {
		http.Error(w, "improper date header", http.StatusBadRequest)
		return
	}
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
	// Inputs are not secret, so this doesn't have to be constant time
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
			algo := strings.ToLower(sigVal)
			if algo != "rsa-sha256" && algo != "hs2019" {
				http.Error(w, "unsupported signature algorithm",
					http.StatusBadRequest)
				return
			}
		case "headers":
			// headers are always lowercase in signature
			// check if the sorted headers list of each are equal
			s1 := strings.Split(sigVal, " ")
			s2 := strings.Split(SigStringHeaders, " ")
			sort.Strings(s1)
			sort.Strings(s2)
			if slices.Equal(s1, s2) {
				http.Error(w, "bad signature headers", http.StatusBadRequest)
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

	requestJson := make(map[string]interface{})
	err = json.Unmarshal(followReqBody, requestJson)
	if err != nil {
		http.Error(w, "bad json syntax: "+err.Error(), http.StatusBadRequest)
		return
	}
	if requestJson["type"] != "Follow" {
		http.Error(w, "unsupported operation", http.StatusNotImplemented)
		return
	}
	// fetch user object
	actor, ok1 := requestJson["actor"]
	actorURL, ok2 := actor.(string)
	if !ok1 || !ok2 {
		http.Error(w, "no actor found", http.StatusBadRequest)
		return
	}
	keyURL, _, _ := strings.Cut(keyID, "#")
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
	actorJson := make(map[string]interface{})
	err = json.NewDecoder(resp.Body).Decode(&actorJson)
	if err != nil {
		http.Error(w, "bad json syntax: "+err.Error(), http.StatusBadRequest)
		return
	}
	publicKeyJSON, ok1 := actorJson["publicKey"]
	publicKeyJSONMap, ok2 := publicKeyJSON.(map[string]interface{})
	publicKeyPEM, ok3 := publicKeyJSONMap["publicKeyPem"]
	publicKeyPEMStr, ok4 := publicKeyPEM.(string)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		http.Error(w, "no actor public key found", http.StatusBadRequest)
		return
	}
	publicBlock, _ := pem.Decode([]byte(publicKeyPEMStr))
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
	signingString := getSigningString(r, SigStringHeaders)

	hashed := sha256.Sum256([]byte(signingString))
	log.Print("signing string:", signingString)

	err = rsa.VerifyPKCS1v15(rsaPublicKey, crypto.SHA256, hashed[:], sigBytes)
	if err != nil {
		http.Error(w, "signature did not match digest "+err.Error(),
			http.StatusUnauthorized)
		return
	}

	actorInbox, ok1 := actorJson["inbox"]
	actorInboxStr, ok2 := actorInbox.(string)
	if !ok1 || !ok2 {
		http.Error(w, "no actor inbox", http.StatusBadRequest)
		return
	}
	actorName, ok1 := actorJson["name"]
	actorNameStr, ok2 := actorName.(string)
	if !ok1 || !ok2 {
		http.Error(w, "no actor name", http.StatusBadRequest)
		return
	}
	parsedURL, _ := url.Parse(actorURL)
	actorAt := actorNameStr + "@" + parsedURL.Host
	log.Println("actor:", actorAt)
	go func() {
		err := AcceptRequest(followReqBody, actorAt, actorInboxStr)
		if err != nil {
			log.Println(err)
		}
	}()

	w.Write(nil)
}

func AcceptRequest(followReqBody []byte, actorAt, actorInboxURL string) error {
	// {
	// 	"@context": "https://www.w3.org/ns/activitystreams",
	// 	"id": "https://maho.dev/@blog#accepts/follows/mictlan@mastodon.social",
	// 	"type": "Accept",
	// 	"actor": "https://maho.dev/@blog",
	// 	"object": {
	// 	  "@context": "https://www.w3.org/ns/activitystreams",
	// 	  "id": "https://mastodon.social/64527582-3605-4d19-ac99-6715df3b0707",
	// 	  "type": "Follow",
	// 	  "actor": "https://mastodon.social/users/mictlan",
	// 	  "object": "https://maho.dev/@blog"
	// 	}
	//  }
	payload := fmt.Sprintf(`{
	"@context": "https://www.w3.org/ns/activitystreams",
	"id": "https://max-blog.koyeb.app/users/@blog#accepts/follows/%s",
 	"type": "Accept",
 	"actor": "https://max-blog.koyeb.app/users/@blog",
	"object": %s%s`, actorAt, followReqBody, "\n}\n")

	log.Println("Payload:", payload)

	// post to actor inbox a message like above ^^^
	r, err := http.NewRequest("POST", actorInboxURL, strings.NewReader(payload))
	if err != nil {
		return err
	}
	// first, compose headers
	r.Header["Date"] = []string{time.Now().UTC().Format(http.TimeFormat)}
	r.Header["Content-Type"] = []string{"application/activity+json; charset=utf-8"}
	digest := sha256.Sum256([]byte(payload))
	digestBase64 := base64.StdEncoding.EncodeToString(digest[:])
	r.Header["Digest"] = []string{"SHA-256=" + digestBase64}

	signingString := getSigningString(r, SigStringHeaders)
	log.Println("signing string 2:", signingString)

	// read PKCIS private key
	privKeyPEM := os.Getenv("AP_PRIVATE_KEY")
	if privKeyPEM == "" {
		return errors.New("no private key found in environment")
	}
	// Convert to PEM block
	privBlock, _ := pem.Decode([]byte(privKeyPEM))
	if privBlock == nil || privBlock.Type != "PRIVATE KEY" {
		return errors.New("failed to decode private key from PEM block")
	}
	// Parse the private key from the block
	privKey, err := x509.ParsePKCS8PrivateKey(privBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key type: %w", err)
	}
	// Check the type of the key
	privKeyRSA, ok := privKey.(*rsa.PrivateKey)
	if !ok {
		return fmt.Errorf("invalid key type: %s", reflect.TypeOf(privKey))
	}

	// Sign header string with PKCIS private key
	hashedHdrs := sha256.Sum256([]byte(signingString))
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, privKeyRSA, crypto.SHA256,
		hashedHdrs[:])
	if err != nil {
		return fmt.Errorf("signing error: %w", err)
	}
	sigBase64 := base64.StdEncoding.EncodeToString(sigBytes)

	r.Header["Signature"] = []string{
		fmt.Sprintf(`keyId="%s",algorithm="%s",headers="%s",signature="%s"`,
			"https://max-blog.koyeb.app/ap/users/@blog#main-key",
			"rsa-sha256",
			SigStringHeaders,
			sigBase64,
		),
	}
	log.Println("Signature:", r.Header["Signature"][0])

	resp, err := (&http.Client{}).Do(r)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("instance did not accept message: %v", resp)
	}
	return nil
}

func getSigningString(r *http.Request, sigHeaders string) string {
	var outStr strings.Builder
	hdrList := strings.Split(sigHeaders, " ")
	for i, hdr := range hdrList {
		switch hdr {
		case "host":
			outStr.WriteString(hdr + ": " + r.Host)
		case "date", "digest", "content-type":
			outStr.WriteString(hdr + ": " + r.Header.Get(hdr))
		case "(request-target)":
			outStr.WriteString(hdr + ": " + strings.ToLower(r.Method) + " " +
				r.URL.Path)
		default:
			// not supporting any other headers for now
		}
		if i != len(hdrList)-1 {
			outStr.WriteByte('\n')
		}
	}
	return outStr.String()
}
