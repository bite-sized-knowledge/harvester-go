package fetcher

// SanitizeXML removes illegal XML 1.0 control characters from raw bytes.
// Valid: #x9 (tab), #xA (LF), #xD (CR), #x20 and above.
func SanitizeXML(data []byte) []byte {
	result := make([]byte, 0, len(data))
	for _, b := range data {
		if b == 0x09 || b == 0x0A || b == 0x0D || b >= 0x20 {
			result = append(result, b)
		}
	}
	return result
}
