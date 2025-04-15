package ap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

var ErrUnauthorized = errors.New(http.StatusText(http.StatusUnauthorized))
var ErrNotImplemented = errors.New(http.StatusText(http.StatusNotImplemented))
var ErrBadRequest = errors.New(http.StatusText(http.StatusBadRequest))

type LambdaRequest = events.APIGatewayProxyRequest
type LambdaResponse = events.APIGatewayProxyResponse

type FollowServiceRequest struct {
	FollowObj string
	Actor     []byte
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

func GetActorAt(actor *Actor) string {
	// Actor name and inbox should be pre-validated
	parsedURL, _ := url.Parse(actor.Id)
	return actor.Name + "@" + parsedURL.Host
}
