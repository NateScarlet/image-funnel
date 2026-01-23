package main

import (
	"crypto/rand"
	"encoding/base64"
)

func mustGenerateRandomSecretKey() string {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}
