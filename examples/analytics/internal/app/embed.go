package app

import _ "embed"

// Script is the tracking script served at /script.js.
//
//go:embed script.js
var Script []byte
