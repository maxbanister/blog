package ap

import (
	"encoding/json"
	"errors"
	"fmt"
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
	PublicKey         *struct {
		PublicKeyPEM string `json:"publicKeyPem"`
	} `json:"publicKey,omitempty" firestore:",omitempty"`
	Icon interface{} `json:"icon"`
}

type InnerReplies struct {
	Id    string `json:"id"`
	Items []any  `json:"items"`
}

type Reply struct {
	Id           string       `json:"id"`
	Type         string       `json:"type,omitempty" firestore:",omitempty"`
	InReplyTo    any          `json:"inReplyTo,omitempty" firestore:",omitempty"`
	Published    string       `json:"published,omitempty" firestore:",omitempty"`
	Updated      string       `json:"updated,omitempty" firestore:",omitempty"`
	URL          string       `json:"url,omitempty" firestore:",omitempty"`
	AttributedTo string       `json:"attributedTo,omitempty" firestore:",omitempty"`
	To           []string     `json:"to,omitempty" firestore:",omitempty"`
	Cc           []string     `json:"cc,omitempty" firestore:",omitempty"`
	Content      string       `json:"content,omitempty" firestore:",omitempty"`
	Replies      InnerReplies `json:"replies"`
	Actor        *Actor       `json:"actor,omitempty" firestore:",omitempty"`
}

type LikeOrShare struct {
	Id     string `json:"id"`
	URL    string `json:"url"`
	Object string `json:"object"`
	Actor  *Actor `json:"actor"`
}

type LikeOrShareContainer struct {
	Id    string        `json:"id"`
	Items []LikeOrShare `json:"items"`
}

const SupportedSigHeaders = "host date digest content-type (request-target)"
const FetchSigHeaders = "host date digest (request-target)"

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

func GetObject(value any) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if ok {
		return object, nil
	}
	objectURI, ok := value.(string)
	if !ok {
		return nil, errors.New("unknown object type")
	}

	respBody, err := RequestAuthorized("GET", "", objectURI)
	if err != nil {
		return nil, fmt.Errorf("could not fetch object: %w", err)
	}

	err = json.Unmarshal(respBody, &object)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal object body: %w", err)
	}

	return object, nil
}

func GetLinkOrObjectID(object any) (objectID string) {
	if objectMap, ok := object.(map[string]any); ok {
		objectID, _ = objectMap["id"].(string)
	} else {
		objectID, _ = object.(string)
	}
	return objectID
}
