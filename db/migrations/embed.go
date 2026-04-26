package migrations

import "embed"

// Files contains the SQL migrations used by the service at startup.
//
//go:embed *.sql
var Files embed.FS
