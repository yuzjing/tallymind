// internal/crypto/hmac.go
package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 16 位签名 (64 位熵)，兼顾 URL 短小美观与极高安全性
const DefaultTokenSignLen = 16

// Sign 生成 HMAC-SHA256 签名 (截取指定长度)
func Sign(secret, data string, length int) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	fullSign := hex.EncodeToString(h.Sum(nil))

	if length > 0 && length <= len(fullSign) {
		return fullSign[:length]
	}
	return fullSign
}

// Verify 恒定时间比对签名 (防时序旁路攻击)
func Verify(secret, data, expectedSign string) bool {
	actualSig := Sign(secret, data, len(expectedSign))
	return hmac.Equal([]byte(actualSig), []byte(expectedSign))
}

// GenerateSignedToken 生成带时间戳与有效期的防篡改 Token (格式: timestamp.signature)
func GenerateSignedToken(secret, playload string, ttl time.Duration, signLen int) string {
	if signLen <= 0 {
		signLen = DefaultTokenSignLen
	}
	expireAt := time.Now().Add(ttl).Unix()
	data := fmt.Sprintf("%s:%d", playload, expireAt)
	sign := Sign(secret, data, signLen)
	return fmt.Sprintf("%d.%s", expireAt, sign)
}

// VerifySignedToken 校验 Token 签名是否合法且未过期
func VerifySignedToken(secret, playload, token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	expireAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > expireAt {
		return false
	}

	data := fmt.Sprintf("%s:%d", playload, expireAt)
	return Verify(secret, data, parts[1])
}
