package handlers

import "crypto/sha1"

// sha1Sum wraps crypto/sha1
func sha1Sum(b []byte) []byte {
	h := sha1.New()
	h.Write(b)
	return h.Sum(nil)
}
