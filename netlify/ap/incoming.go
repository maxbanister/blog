package ap

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	. "github.com/maxbanister/blog/netlify/util"
)

func RecvActivity(r *LambdaRequest, requestJSON map[string]any) (*Actor, error) {
	reqDate, err := time.Parse(http.TimeFormat, r.Headers["date"])
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}
	if time.Since(reqDate) >= 2*time.Hour {
		return nil, fmt.Errorf("%w: date header too old", ErrBadRequest)
	}

	err = checkDigest(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}

	sigBytes, keyID, sigStrHdrs, err := getSigHeaderParts(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}

	// fetch actor object
	actorProperty, exists := requestJSON["actor"]
	if !exists {
		return nil, fmt.Errorf("%w: no actor found", ErrBadRequest)
	}
	if actorURL, isURL := actorProperty.(string); isURL {
		keyURL, _, _ := strings.Cut(keyID, "#")
		if keyURL != actorURL {
			return nil, fmt.Errorf("%w: actor does not match key in signature",
				ErrBadRequest)
		}
	}
	actor, err := FetchActorAuthorized(requestJSON["actor"])
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}
	rsaPublicKey, err := getActorPubKey(actor)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnauthorized, err)
	}
	// erase the public key so we don't accidentally bloat our stored objects
	actor.PublicKey = nil

	h, m, p := r.Headers["host"], r.HTTPMethod, r.Path
	signingString := getSigningString(h, m, p, sigStrHdrs, r.Headers)

	hashed := sha256.Sum256([]byte(signingString))
	fmt.Println("signing string:", signingString)

	err = rsa.VerifyPKCS1v15(rsaPublicKey, crypto.SHA256, hashed[:], sigBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: signature did not match digest: %s",
			ErrUnauthorized, err.Error())
	}

	return actor, nil
}

func getSigHeaderParts(r *LambdaRequest) ([]byte, string, string, error) {
	signatureHeader := r.Headers["signature"]
	if signatureHeader == "" {
		return nil, "", "", errors.New("no signature header")
	}
	var keyID, sigBase64, sigStrHdrs string
	for _, sig := range strings.Split(signatureHeader, ",") {
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
				return nil, "", "", errors.New("unsupported signature algorithm")
			}
		case "headers":
			// headers are always lowercase in signature
			// check if the headers values of each are equal
			supportedHdrs := strings.Split(SupportedSigHeaders, " ")
			for _, hdrStr := range strings.Split(sigVal, " ") {
				if !slices.Contains(supportedHdrs, hdrStr) {
					return nil, "", "", errors.New("bad signature headers")
				}
			}
			sigStrHdrs = sigVal
		}
	}
	if keyID == "" || sigBase64 == "" {
		return nil, "", "", errors.New("invalid signature")
	}
	sigBytes, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return nil, "", "", fmt.Errorf("couldn't decode base64 signature: %w", err)
	}

	return sigBytes, keyID, sigStrHdrs, nil
}

func getActorPubKey(actor *Actor) (*rsa.PublicKey, error) {
	var publicKeyPEM string
	if actor.PublicKey != nil {
		publicKeyPEM = actor.PublicKey.PublicKeyPEM
	}

	publicBlock, _ := pem.Decode([]byte(publicKeyPEM))
	if publicBlock == nil || publicBlock.Type != "PUBLIC KEY" {
		return nil, errors.New("failed to decode public key")
	}
	publicKey, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse cert: %w", err)
	}
	rsaPublicKey, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		errMsg := fmt.Sprintf("invalid key type: %s", reflect.TypeOf(publicKey))
		return nil, errors.New(errMsg)
	}

	return rsaPublicKey, nil
}

func FetchActorAuthorized(actorData any) (*Actor, error) {
	var respBody []byte

	switch actorVal := actorData.(type) {
	case string:
		var err error
		respBody, err = RequestAuthorized("GET", "", actorVal)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
		}

	case map[string]any:
		// it is rare that the actor is embedded in the request, so we can shirk
		// performance and convert the map back to JSON and redecode as a struct
		respBody, _ = json.Marshal(actorVal)

	default:
		return nil, fmt.Errorf("%w: unknown actor type", ErrBadRequest)
	}

	var actor Actor
	err := json.Unmarshal(respBody, &actor)
	if err != nil {
		return nil, fmt.Errorf("bad json syntax: %s", err.Error())
	}
	if actor.PublicKey == nil || actor.PublicKey.PublicKeyPEM == "" {
		return nil, errors.New("no actor public key found")
	}
	if actor.Inbox == "" {
		return nil, errors.New("no actor inbox found")
	}
	if actor.Name == "" && actor.PreferredUsername == "" {
		return nil, errors.New("no actor name found")
	}
	if actorIcon, ok := actor.Icon.(map[string]any); ok {
		actor.Icon = actorIcon["url"]
	}

	return &actor, nil
}

func checkDigest(r *LambdaRequest) error {
	digest := r.Headers["digest"]
	if digest == "" {
		return errors.New("no digest header")
	}
	digestAlgo, digestBase64, hasSep := strings.Cut(digest, "=")
	if !hasSep {
		return errors.New("malformed digest header")
	}
	if strings.ToLower(digestAlgo) != "sha-256" {
		return errors.New("unsupported digest algorithm")
	}
	digestBytes, err := base64.StdEncoding.DecodeString(digestBase64)
	if err != nil {
		return fmt.Errorf("couldn't decode base64 digest: %w", err)
	}
	reqBodyHash := sha256.Sum256([]byte(r.Body))
	// inputs are not secret, so this doesn't have to be constant time
	if !bytes.Equal(reqBodyHash[:], digestBytes) {
		return errors.New("digest didn't match message body")
	}

	return nil
}
