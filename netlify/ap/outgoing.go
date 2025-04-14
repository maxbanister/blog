package ap

import (
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"
)

func SendActivity(payload string, actor *Actor) {
	fmt.Println("Payload:", payload)

	// post to actor inbox a message
	actorInbox := actor.Inbox
	r, err := http.NewRequest("POST", actorInbox, strings.NewReader(payload))
	if err != nil {
		fmt.Println("couldn't post to actor inbox:", err.Error())
		return
	}
	// first, compose headers
	r.Header["date"] = []string{time.Now().UTC().Format(http.TimeFormat)}
	r.Header["content-type"] = []string{"application/activity+json; charset=utf-8"}
	digest := sha256.Sum256([]byte(payload))
	digestBase64 := base64.StdEncoding.EncodeToString(digest[:])
	r.Header["digest"] = []string{"SHA-256=" + digestBase64}

	h, m, p := r.Host, r.Method, r.URL.Path
	signingString := getSigningString(h, m, p, SigStringHeaders, r.Header)
	fmt.Println("signing string 2:", signingString)

	privKeyRSA, err := getPrivKey()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	// Sign header string with PKCIS private key
	hashedHdrs := sha256.Sum256([]byte(signingString))
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, privKeyRSA, crypto.SHA256,
		hashedHdrs[:])
	if err != nil {
		fmt.Println("signing error:", err.Error())
		return
	}
	sigBase64 := base64.StdEncoding.EncodeToString(sigBytes)

	r.Header["Signature"] = []string{
		fmt.Sprintf(`keyId="%s",algorithm="%s",headers="%s",signature="%s"`,
			"https://maxscribes.netlify.app/ap/user/blog#main-key",
			"rsa-sha256",
			SigStringHeaders,
			sigBase64,
		),
	}
	fmt.Println("Signature:", r.Header["Signature"][0])

	resp, err := (&http.Client{}).Do(r)
	if err != nil {
		fmt.Println("error sending AcceptFollow:", err.Error())
		return
	}
	respBody, _ := io.ReadAll(resp.Body)
	fmt.Println(resp.StatusCode, string(respBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Printf("instance did not accept activity: %v\n", resp)
		return
	}
}

func getPrivKey() (*rsa.PrivateKey, error) {
	// read PKCIS private key
	privKeyBase64 := os.Getenv("AP_PRIVATE_KEY")
	fmt.Println(privKeyBase64)
	decoded, err := base64.StdEncoding.DecodeString(privKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("could not decode base64 priv key: %w", err)
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, fmt.Errorf("gzip header invalid: %w", err)
	}
	defer gzipReader.Close()
	privKeyPEM, err := io.ReadAll(gzipReader)
	if err != nil {
		fmt.Println("could not decode gzipped data: %w", err)
	}
	if len(privKeyPEM) == 0 {
		return nil, errors.New("private key empty")
	}
	privKeyPEM = bytes.ReplaceAll(privKeyPEM, []byte("\\n"), []byte{'\n'})

	// Convert to PEM block
	privBlock, _ := pem.Decode([]byte(privKeyPEM))
	if privBlock == nil || privBlock.Type != "PRIVATE KEY" {
		return nil, errors.New("failed to decode private key from PEM block")
	}
	// Parse the private key from the block
	privKey, err := x509.ParsePKCS8PrivateKey(privBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key type: %w", err)
	}
	// Check the type of the key
	privKeyRSA, ok := privKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("invalid key type: %s", reflect.TypeOf(privKey))
	}

	return privKeyRSA, nil
}
