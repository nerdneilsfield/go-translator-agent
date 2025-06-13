package epub

import (
	"crypto/rand"
	"encoding/hex"
)

// generateDocumentID 生成文档 ID
func generateDocumentID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}