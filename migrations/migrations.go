// Package migrations embeds the platform schema so lp-server can apply it on
// startup. Files are applied in lexical order; never edit an applied file.
package migrations

import "embed"

//go:embed *.up.sql
var FS embed.FS
