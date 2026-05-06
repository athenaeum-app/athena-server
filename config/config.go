package config

import (
	"crypto/sha256"
	"os"
	"strconv"
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
	if p := os.Getenv("PASSWORD"); p != "" {
		return p
	}
	return "viewer"
}

func UploadLimit() int64 {
	limitStr := os.Getenv("MAX_UPLOAD_MB")

	if limitStr == "" {
		return 500 << 20
	}

	limitMB, err := strconv.ParseInt(limitStr, 10, 64)

	if err != nil || limitMB <= 0 {
		return 500 << 20
	}

	return limitMB << 20
}
