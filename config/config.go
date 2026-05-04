package config

import (
	"crypto/sha256"
	"os"
)

func JWTSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}

	hash := sha256.Sum256([]byte(AdminPassword() + "athena-secure-salt"))
	return hash[:]
}

func AdminPassword() string {
	if p := os.Getenv("ADMIN_PASSWORD"); p != "" {
		return p
	}
	return "admin"
}

func ViewerPassword() string {
	if p := os.Getenv("VIEWER_PASSWORD"); p != "" {
		return p
	}
	return "viewer"
}
