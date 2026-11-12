//go:build tools

// Pins the NATS server module for go mod vendor. nats-server is a main package, so the import
// cannot be part of the default build; the Dockerfile builds it by import path from vendor/.

package main

import _ "github.com/nats-io/nats-server/v2"
