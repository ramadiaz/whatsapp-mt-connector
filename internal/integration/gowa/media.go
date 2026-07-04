package gowa

import (
	"fmt"
	"net/http"
)

var allowedMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

func ValidateMediaMIME(mimeType string) error {
	for k := range allowedMIMETypes {
		if mimeType == k || (len(mimeType) > len(k) && mimeType[:len(k)] == k) {
			return nil
		}
	}
	return fmt.Errorf("unsupported media type: %s", mimeType)
}

func DetectMIME(data []byte) string {
	return http.DetectContentType(data)
}
