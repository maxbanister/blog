package ap

import (
	"net/http"
	"net/url"
	"strings"
)

type FollowServiceRequest struct {
	FollowObj string
	Actor     []byte
}

type Actor struct {
	Id                string `json:"id"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferredUsername"`
	Inbox             string `json:"inbox"`
	PublicKey         struct {
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

func GetActorAt(actor *Actor) string {
	// Actor name and inbox should be pre-validated
	parsedURL, _ := url.Parse(actor.Id)
	if actor.PreferredUsername != "" {
		return actor.PreferredUsername + "@" + parsedURL.Host
	}
	return actor.Name + "@" + parsedURL.Host
}
