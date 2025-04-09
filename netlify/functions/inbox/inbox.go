package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

type LambdaRequest = events.APIGatewayProxyRequest
type LambdaResponse = events.APIGatewayProxyResponse
type LambdaCtx = lambdacontext.LambdaContext

const SigStringHeaders = "host date digest content-type (request-target)"

var ErrUnauthorized = errors.New(http.StatusText(http.StatusUnauthorized))
var ErrNotImplemented = errors.New(http.StatusText(http.StatusNotImplemented))
var ErrBadRequest = errors.New(http.StatusText(http.StatusBadRequest))

func main() {
	lambda.Start(handleInbox)
}

func handleInbox(request LambdaRequest) (*LambdaResponse, error) {
	fmt.Println("Headers:", request.Headers)

	requestJSON := make(map[string]any)
	err := json.Unmarshal([]byte(request.Body), &requestJSON)
	if err != nil {
		return getLambdaResp(fmt.Errorf(
			"%w: bad json syntax: %s", ErrBadRequest, err.Error()))
	}

	switch requestJSON["type"] {
	case "Follow":
		actorJSON, err := handleFollow(&request, requestJSON)
		if err != nil {
			return getLambdaResp(err)
		}
		// fire and forget
		go AcceptRequest(request.Body, actorJSON)
		return getLambdaResp(nil)
	default:
		return getLambdaResp(fmt.Errorf(
			"%w: unsupported operation", ErrNotImplemented))
	}
}

func getLambdaResp(err error) (*LambdaResponse, error) {
	var code int
	if errors.Is(err, ErrUnauthorized) {
		code = http.StatusUnauthorized
	} else if errors.Is(err, ErrBadRequest) {
		code = http.StatusBadRequest
	} else if errors.Is(err, ErrNotImplemented) {
		code = http.StatusNotImplemented
	} else {
		code = http.StatusOK
	}
	return &events.APIGatewayProxyResponse{
		StatusCode: code,
		Body:       err.Error(),
	}, nil
}

func handleFollow(r *LambdaRequest, requestJSON map[string]any) (map[string]any, error) {
	reqDate, err := time.Parse(http.TimeFormat, r.Headers["Date"])
	if err != nil || time.Since(reqDate) >= 2*time.Hour {
		return nil, fmt.Errorf("%w: date header too old", ErrBadRequest)
	}

	err = checkDigest(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}

	sigBytes, keyID, err := getSigHeaderParts(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}

	// fetch actor object
	actor, ok1 := requestJSON["actor"]
	actorURL, ok2 := actor.(string)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("%w: no actor found", ErrBadRequest)
	}

	keyURL, _, _ := strings.Cut(keyID, "#")
	if keyURL != actorURL {
		return nil, fmt.Errorf("%w: actor does not match key in signature",
			ErrBadRequest)
	}
	req, err := http.NewRequest("GET", actorURL, nil)
	req.Header.Set("Accept", `application/ld+json; profile="https://www.w3.org/ns/activitystreams`)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}
	actorJSON := make(map[string]any)
	err = json.NewDecoder(resp.Body).Decode(&actorJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: bad json syntax: %s", ErrBadRequest,
			err.Error())
	}
	if err = validateActor(actorJSON); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}
	rsaPublicKey, err := getActorPubKey(actorJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnauthorized, err)
	}

	h, m, p := r.RequestContext.DomainName, r.HTTPMethod, r.Path
	signingString := getSigningString(h, m, p, SigStringHeaders, r.Headers)

	hashed := sha256.Sum256([]byte(signingString))
	log.Println("signing string:", signingString)

	err = rsa.VerifyPKCS1v15(rsaPublicKey, crypto.SHA256, hashed[:], sigBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: signature did not match digest: %s",
			ErrUnauthorized, err.Error())
	}

	return actorJSON, nil
}

func checkDigest(r *LambdaRequest) error {
	digest := r.Headers["Digest"]
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
	// Inputs are not secret, so this doesn't have to be constant time
	if !bytes.Equal(reqBodyHash[:], digestBytes) {
		return errors.New("digest didn't match message body")
	}

	return nil
}

func getSigHeaderParts(r *LambdaRequest) ([]byte, string, error) {
	signatureHeader := r.Headers["Signature"]
	if signatureHeader == "" {
		return nil, "", errors.New("no signature header")
	}
	var keyID, sigBase64 string
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
				return nil, "", errors.New("unsupported signature algorithm")
			}
		case "headers":
			// headers are always lowercase in signature
			// check if the sorted headers list of each are equal
			s1 := strings.Split(sigVal, " ")
			s2 := strings.Split(SigStringHeaders, " ")
			sort.Strings(s1)
			sort.Strings(s2)
			if slices.Equal(s1, s2) {
				return nil, "", errors.New("bad signature headers")
			}
		}
	}
	if keyID == "" || sigBase64 == "" {
		return nil, "", errors.New("invalid signature")
	}
	sigBytes, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return nil, "", fmt.Errorf("couldn't decode base64 signature: %w", err)
	}

	return sigBytes, keyID, nil
}

func validateActor(actorJSON map[string]any) error {
	publicKeyJSON, ok1 := actorJSON["publicKey"]
	publicKeyJSONMap, ok2 := publicKeyJSON.(map[string]any)
	publicKeyPEM, ok3 := publicKeyJSONMap["publicKeyPem"]
	_, ok4 := publicKeyPEM.(string)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return errors.New("no actor public key found")
	}

	actorInbox, ok1 := actorJSON["inbox"]
	_, ok2 = actorInbox.(string)
	if !ok1 || !ok2 {
		return errors.New("no actor inbox")
	}

	actorName, ok1 := actorJSON["name"]
	_, ok2 = actorName.(string)
	if !ok1 || !ok2 {
		return errors.New("no actor name")
	}

	return nil
}

func getActorPubKey(actorJson map[string]any) (*rsa.PublicKey, error) {
	publicKeyJSON := actorJson["publicKey"].(map[string]any)
	publicKeyPEM := publicKeyJSON["publicKeyPem"].(string)

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

func AcceptRequest(followReqBody string, actorJSON map[string]any) {
	// Pre-validated actor name and inbox exist and are strings
	parsedURL, _ := url.Parse(actorJSON["id"].(string))
	actorAt := actorJSON["name"].(string) + "@" + parsedURL.Host
	log.Println("actor:", actorAt)

	payload := fmt.Sprintf(`{
	"@context": "https://www.w3.org/ns/activitystreams",
	"id": "https://maxscribes.netlify.app/ap/user/blog#accepts/follows/%s",
 	"type": "Accept",
 	"actor": "https://maxscribes.netlify.app/ap/user/blog",
	"object": %s%s`, actorAt, followReqBody, "\n}\n")

	log.Println("Payload:", payload)

	// post to actor inbox a message
	actorInbox := actorJSON["inbox"].(string)
	r, err := http.NewRequest("POST", actorInbox, strings.NewReader(payload))
	if err != nil {
		log.Println("couldn't post to actor inbox:", err.Error())
		return
	}
	// first, compose headers
	r.Header["Date"] = []string{time.Now().UTC().Format(http.TimeFormat)}
	log.Println("date sent:", r.Header["Date"])
	r.Header["Content-Type"] = []string{"application/activity+json; charset=utf-8"}
	digest := sha256.Sum256([]byte(payload))
	digestBase64 := base64.StdEncoding.EncodeToString(digest[:])
	r.Header["Digest"] = []string{"SHA-256=" + digestBase64}

	h, m, p := r.Host, r.Method, r.URL.Path
	signingString := getSigningString(h, m, p, SigStringHeaders, r.Header)
	log.Println("signing string 2:", signingString)

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
		log.Println("signing error:", err.Error())
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
	log.Println("Signature:", r.Header["Signature"][0])

	resp, err := (&http.Client{}).Do(r)
	if err != nil {
		log.Println("error sending AcceptFollow:", err.Error())
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("instance did not accept message: %v\n", resp)
		return
	}
}

func getPrivKey() (*rsa.PrivateKey, error) {
	// read PKCIS private key
	privKeyPEM := os.Getenv("AP_PRIVATE_KEY")
	if privKeyPEM == "" {
		return nil, errors.New("no private key found in environment")
	}
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

func getSigningString(h, m, p, sigHeaders string, hdrs interface{}) string {
	var outStr strings.Builder
	hdrList := strings.Split(sigHeaders, " ")
	for i, hdr := range hdrList {
		switch hdr {
		case "host":
			outStr.WriteString(hdr + ": " + h)
		case "date", "digest", "content-type":
			// could be from a gostd http request or lambda request
			if sliceHdr, ok := hdrs.(map[string][]string); ok {
				outStr.WriteString(hdr + ": " + strings.Join(sliceHdr[hdr], ""))
			} else if hdrs, ok := hdrs.(map[string]string); ok {
				outStr.WriteString(hdr + ": " + hdrs[hdr])
			}
		case "(request-target)":
			outStr.WriteString(hdr + ": " + strings.ToLower(m) + " " + p)
		default:
			// not supporting any other headers for now
		}
		if i != len(hdrList)-1 {
			outStr.WriteByte('\n')
		}
	}
	return outStr.String()
}
