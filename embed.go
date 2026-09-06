package corum

import "embed"

// Assets contains the agent toolkit and schemas installed into Corum vaults.
//
//go:embed all:agent-kit all:schemas
var Assets embed.FS
