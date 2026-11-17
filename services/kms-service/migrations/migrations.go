// Package migrations embeds the kms-service SQL migrations.
package migrations

import "embed"

// FS holds every *.up.sql file in this directory.
//
//go:embed *.sql
var FS embed.FS
