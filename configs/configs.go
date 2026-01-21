// Package configs embeds configuration files for use at runtime.
package configs

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

// BaseDockerfile contains the embedded base.Dockerfile content.
// This allows ccells to build the base image from any directory.
//
//go:embed base.Dockerfile
var BaseDockerfile []byte

// BaseDockerfileHash returns a 12-character hash of the embedded Dockerfile.
// This is used for content-based image tagging.
func BaseDockerfileHash() string {
	hash := sha256.Sum256(BaseDockerfile)
	return hex.EncodeToString(hash[:])[:12]
}
