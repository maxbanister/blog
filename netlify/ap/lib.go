package ap

import (
	"bytes"
	"context"
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
	"net/http"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

var ErrUnauthorized = errors.New(http.StatusText(http.StatusUnauthorized))
var ErrNotImplemented = errors.New(http.StatusText(http.StatusNotImplemented))
var ErrBadRequest = errors.New(http.StatusText(http.StatusBadRequest))

type LambdaRequest = events.APIGatewayProxyRequest
type LambdaResponse = events.APIGatewayProxyResponse

type ReplyServiceRequest struct {
	ReplyObj string
	Actor    []byte
}

type Actor struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Inbox     string `json:"inbox"`
	PublicKey struct {
		PublicKeyPEM string `json:"publicKeyPem"`
	} `json:"publicKey"`
}

const SigStringHeaders = "host date digest content-type (request-target)"

func HandleFollow(r *LambdaRequest, requestJSON map[string]any) (*Actor, error) {
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

	sigBytes, keyID, err := getSigHeaderParts(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}

	// fetch actor object
	actorProperty, ok1 := requestJSON["actor"]
	actorURL, ok2 := actorProperty.(string)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("%w: no actor found", ErrBadRequest)
	}

	keyURL, _, _ := strings.Cut(keyID, "#")
	if keyURL != actorURL {
		return nil, fmt.Errorf("%w: actor does not match key in signature",
			ErrBadRequest)
	}
	actor, err := fetchActor(actorURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}
	rsaPublicKey, err := getActorPubKey(actor)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnauthorized, err)
	}

	h, m, p := r.Headers["host"], r.HTTPMethod, r.Path
	signingString := getSigningString(h, m, p, SigStringHeaders, r.Headers)

	hashed := sha256.Sum256([]byte(signingString))
	fmt.Println("signing string:", signingString)

	err = rsa.VerifyPKCS1v15(rsaPublicKey, crypto.SHA256, hashed[:], sigBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: signature did not match digest: %s",
			ErrUnauthorized, err.Error())
	}

	return actor, nil
}

func fetchActor(actorURL string) (*Actor, error) {
	req, err := http.NewRequest("GET", actorURL, nil)
	req.Header.Set("Accept",
		`application/ld+json; profile="https://www.w3.org/ns/activitystreams`)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadRequest, err)
	}

	var actor Actor
	err = json.NewDecoder(resp.Body).Decode(&actor)
	if err != nil {
		return nil, fmt.Errorf("bad json syntax: %s", err.Error())
	}
	if actor.PublicKey.PublicKeyPEM == "" {
		return nil, errors.New("no actor public key found")
	}
	if actor.Inbox == "" {
		return nil, errors.New("no actor inbox found")
	}
	if actor.Name == "" {
		return nil, errors.New("no actor name found")
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
	// Inputs are not secret, so this doesn't have to be constant time
	if !bytes.Equal(reqBodyHash[:], digestBytes) {
		return errors.New("digest didn't match message body")
	}

	return nil
}

func getSigHeaderParts(r *LambdaRequest) ([]byte, string, error) {
	signatureHeader := r.Headers["signature"]
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
			// check if the headers values of each are equal
			sigStrHdrs := strings.Split(SigStringHeaders, " ")
			for _, hdrStr := range strings.Split(sigVal, " ") {
				if !slices.Contains(sigStrHdrs, hdrStr) {
					return nil, "", errors.New("bad signature headers")
				}
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

func getActorPubKey(actor *Actor) (*rsa.PublicKey, error) {
	publicKeyPEM := actor.PublicKey.PublicKeyPEM

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

func AcceptRequest(hostSite, followReqBody string, actor *Actor) {
	// Pre-validated actor name and inbox
	parsedURL, _ := url.Parse(actor.Id)
	actorAt := actor.Name + "@" + parsedURL.Host
	fmt.Println("Actor:", actorAt)

	payload := fmt.Sprintf(`{
	"@context": "https://www.w3.org/ns/activitystreams",
	"id": "%s/ap/user/blog#accepts/follows/%s",
 	"type": "Accept",
 	"actor": "https://%s/ap/user/blog",
	"object": %s%s`, hostSite, actorAt, hostSite, followReqBody, "\n}\n")

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
	fmt.Println(resp.StatusCode, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Printf("instance did not accept message: %v\n", resp)
		return
	}
}

func getPrivKey() (*rsa.PrivateKey, error) {
	// read PKCIS private key
	privKeyPEM := os.Getenv("AP_PRIVATE_KEY")
	if privKeyPEM == "" {
		return nil, errors.New("no private key found in environment")
	}
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

func getSigningString(host, method, path, sigHeaders string, hdrs any) string {
	var outStr strings.Builder
	hdrList := strings.Split(sigHeaders, " ")
	for i, hdr := range hdrList {
		switch hdr {
		case "host":
			outStr.WriteString(hdr + ": " + host)
		case "date", "digest", "content-type":
			// could be from a gostd http request or lambda request
			if sliceHdr, ok := hdrs.(http.Header); ok {
				outStr.WriteString(hdr + ": " + strings.Join(sliceHdr[hdr], ""))
			} else if hdrs, ok := hdrs.(map[string]string); ok {
				outStr.WriteString(hdr + ": " + hdrs[hdr])
			}
		case "(request-target)":
			outStr.WriteString(hdr + ": " + strings.ToLower(method) + " " + path)
		default:
			// not supporting any other headers for now
		}
		if i != len(hdrList)-1 {
			outStr.WriteByte('\n')
		}
	}
	return outStr.String()
}

func GetHostSite(ctx context.Context) string {
	var jsonData []byte
	var err error
	lc, ok := lambdacontext.FromContext(ctx)
	if !ok {
		fmt.Println("could not get lambda context")
	} else {
		ccc := lc.ClientContext.Custom
		jsonData, err = base64.StdEncoding.DecodeString(ccc["netlify"])
		if err != nil {
			fmt.Println("could not decode netlify base64:", err)
			return ""
		}
	}
	var netlifyData map[string]string
	err = json.Unmarshal(jsonData, &netlifyData)
	if err != nil {
		fmt.Println("could not decode netlify json:", err)
		return ""
	}
	return netlifyData["site_url"]
}
