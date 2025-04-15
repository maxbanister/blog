package kv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"google.golang.org/api/option"
)

const certURLPrefix = "https://www.googleapis.com/robot/v1/metadata/x509/firebase-adminsdk-fbsvc%40"

func GetFirestoreClient() (*firestore.Client, error) {
	// Use a service account
	serviceAccountJSON := map[string]string{
		"type":                        "service_account",
		"project_id":                  "max-banister-blog",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
	}
	client_email := os.Getenv("GOOGLE_CLIENT_EMAIL")
	priv_key := strings.ReplaceAll(os.Getenv("GOOGLE_PRIV_KEY"), "\\n", "\n")
	_, emailDomain, _ := strings.Cut(client_email, "@")
	serviceAccountJSON["private_key_id"] = os.Getenv("GOOGLE_PRIV_KEY_ID")
	serviceAccountJSON["private_key"] = priv_key
	serviceAccountJSON["client_email"] = client_email
	serviceAccountJSON["client_id"] = os.Getenv("GOOGLE_CLIENT_ID")
	serviceAccountJSON["client_x509_cert_url"] = certURLPrefix + emailDomain
	marshalledSA, err := json.Marshal(serviceAccountJSON)
	if err != nil {
		return nil, fmt.Errorf("could not marshal service account: %w", err)
	}

	ctx := context.Background()
	sa := option.WithCredentialsJSON(marshalledSA)
	app, err := firebase.NewApp(ctx, nil, sa)
	if err != nil {
		return nil, err
	}

	client, err := app.Firestore(ctx)
	if err != nil {
		return nil, err
	}

	return client, nil
}
