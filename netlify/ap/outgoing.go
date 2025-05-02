package ap

import (
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

	. "github.com/maxbanister/blog/netlify/util"
)

func SendActivity(payload string, actor *Actor) error {
	fmt.Println("Payload:", payload)

	// post to actor inbox a message
	actorInbox := actor.Inbox
	r, err := http.NewRequest("POST", actorInbox, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("couldn't post to actor inbox: %w", err)
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
		return err
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
			GetHostSite()+"/ap/user/max#main-key",
			"rsa-sha256",
			SigStringHeaders,
			sigBase64,
		),
	}
	fmt.Println("Signature:", r.Header["Signature"][0])

	resp, err := (&http.Client{}).Do(r)
	if err != nil {
		return fmt.Errorf("error sending AcceptFollow: %w", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	fmt.Println(resp.StatusCode, string(respBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("instance did not accept activity: %v", resp)
	}

	return nil
}

func getPrivKey() (*rsa.PrivateKey, error) {
	// read PKCIS private key
	privKeyPEM := os.Getenv("AP_PRIVATE_KEY")
	privKeyPEM = strings.ReplaceAll(privKeyPEM, "\\n", "\n")

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
