package blog

import _ "embed"

//go:embed public/ap/outbox.json
var OutboxJSON []byte
