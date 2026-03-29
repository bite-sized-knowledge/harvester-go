package hasher

import (
	"crypto/sha1"
	"math/big"
)

const base62Chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

var base62Length = big.NewInt(int64(len(base62Chars)))

func HashToSha1Base62(input string) string {
	hash := sha1.Sum([]byte(input))
	return encodeBase62(hash[:])
}

func encodeBase62(buffer []byte) string {
	value := new(big.Int).SetBytes(buffer)
	if value.Sign() == 0 {
		return ""
	}

	result := make([]byte, 0, 27)
	remainder := new(big.Int)
	for value.Sign() > 0 {
		value.QuoRem(value, base62Length, remainder)
		result = append(result, base62Chars[remainder.Int64()])
	}

	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}
