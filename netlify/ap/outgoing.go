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
	// post a message to actor inbox
	_, err := RequestAuthorized("POST", payload, actor.Inbox)
	return err
}

func RequestAuthorized(method, payload, destURL string) ([]byte, error) {
	//fmt.Println("Payload:", payload)

	r, err := http.NewRequest(method, destURL, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("couldn't post to actor inbox: %w", err)
	}
	// first, compose headers
	var sigHeaders string
	r.Header["date"] = []string{time.Now().UTC().Format(http.TimeFormat)}
	if method == "POST" {
		r.Header["content-type"] = []string{
			"application/activity+json; charset=utf-8",
		}
		sigHeaders = SupportedSigHeaders
	} else if method == "GET" {
		r.Header.Set("accept", `application/ld+json; `+
			`profile="https://www.w3.org/ns/activitystreams`)
		sigHeaders = FetchSigHeaders
	}
	// digest will always be sha256("") for a get request, but the AP spec
	// seems to require it
	digest := sha256.Sum256([]byte(payload))
	digestBase64 := base64.StdEncoding.EncodeToString(digest[:])
	r.Header["digest"] = []string{"SHA-256=" + digestBase64}

	h, m, p := r.Host, r.Method, r.URL.Path
	signingString := getSigningString(h, m, p, sigHeaders, r.Header)
	//fmt.Println("signing string 2:", signingString)

	privKeyRSA, err := getPrivKey()
	if err != nil {
		return nil, err
	}

	// Sign header string with PKCIS private key
	hashedHdrs := sha256.Sum256([]byte(signingString))
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, privKeyRSA, crypto.SHA256,
		hashedHdrs[:])
	if err != nil {
		return nil, fmt.Errorf("signing error: %w", err)
	}
	sigBase64 := base64.StdEncoding.EncodeToString(sigBytes)

	r.Header["Signature"] = []string{
		fmt.Sprintf(`keyId="%s",algorithm="%s",headers="%s",signature="%s"`,
			GetHostSite()+"/ap/user/max#main-key",
			"rsa-sha256",
			sigHeaders,
			sigBase64,
		),
	}
	//fmt.Println("Signature:", r.Header["Signature"][0])

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("error sending activity: %w", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Println(resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("http activity request error: %v", resp)
	}
	fmt.Println(resp.StatusCode, string(respBody))

	return respBody, nil
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
