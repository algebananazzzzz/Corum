package corum

import "embed"

//go:embed agent-kit
var Assets embed.FS

// TrackerStateSchema lets doctor check the tracker.json files agents write by hand.
//
//go:embed schemas/tracker-state.schema.json
var TrackerStateSchema []byte
