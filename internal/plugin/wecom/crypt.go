package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

// WXBizMsgCrypt 企微标准签名校验与 AES-256-CBC 加解密器
type WXBizMsgCrypt struct {
	token          string
	encodingAESKey string
	corpID         string
	aesKey         []byte
}

func NewWXBizMsgCrypt(token, encodingAESKey, corpID string) *WXBizMsgCrypt {
	// 企微 EncodingAESKey 为 43 位字符串，加上 '=' 成为 44 位 Base64，解码后为 32 字节 AES Key
	aesKey, _ := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	return &WXBizMsgCrypt{
		token:          token,
		encodingAESKey: encodingAESKey,
		corpID:         corpID,
		aesKey:         aesKey,
	}
}

// 1. SHA1 签名校验：Sort(token, timestamp, nonce, encrypt) 后计算 SHA1 摘要
func (c *WXBizMsgCrypt) verifySignature(token, timestamp, nonce, msgEncrypt, signature string) bool {
	sl := []string{token, timestamp, nonce, msgEncrypt}
	sort.Strings(sl)
	h := sha1.New()
	h.Write([]byte(strings.Join(sl, "")))
	calculatedSig := fmt.Sprintf("%x", h.Sum(nil))
	return calculatedSig == signature
}

// 2. PKCS#7 反填充算法
func pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("invalid unpad length")
	}
	unpadding := int(data[length-1])
	if unpadding < 1 || unpadding > 32 || length < unpadding {
		return nil, fmt.Errorf("invalid padding size")
	}
	return data[:(length - unpadding)], nil
}

// 3. AES-256-CBC 解密底层核心算法
func (c *WXBizMsgCrypt) decrypt(cipherData []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return nil, err
	}
	if len(cipherData) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// 企微 IV 约定为 aesKey 前 16 字节
	iv := c.aesKey[:16]
	mode := cipher.NewCBCDecrypter(block, iv)

	plainData := make([]byte, len(cipherData))
	mode.CryptBlocks(plainData, cipherData)

	// 去除 PKCS#7 填充
	plainData, err = pkcs7Unpad(plainData)
	if err != nil {
		return nil, err
	}

	// 企微加密明文结构：16 字节随机数 + 4 字节 msgLen (网络大端序) + msg + corpID
	if len(plainData) < 20 {
		return nil, fmt.Errorf("plaintext too short")
	}

	// 读取 4 字节的大端序网络字节流，获取实际消息长度
	msgLen := binary.BigEndian.Uint32(plainData[16:20])
	if len(plainData) < int(20+msgLen) {
		return nil, fmt.Errorf("msg length mismatched")
	}

	// 截取真实消息与 CorpID
	msg := plainData[20 : 20+msgLen]
	receivedCorpID := string(plainData[20+msgLen:])

	// 校验接收到的 CorpID 是否与本地配置匹配，防范伪造攻击
	if receivedCorpID != c.corpID {
		return nil, fmt.Errorf("corpID mismatched: expect %s, got %s", c.corpID, receivedCorpID)
	}

	return msg, nil
}

// VerifyURL 处理企微后台第一次设置 API 接收时的 GET echostr 验证
func (c *WXBizMsgCrypt) VerifyURL(msgSignature, timestamp, nonce, echoStr string) ([]byte, error) {
	// 校验签名
	if !c.verifySignature(c.token, timestamp, nonce, echoStr, msgSignature) {
		return nil, fmt.Errorf("signature verification failed")
	}

	// Base64 解码
	cipherData, err := base64.StdEncoding.DecodeString(echoStr)
	if err != nil {
		return nil, fmt.Errorf("base64 decode echostr error: %w", err)
	}

	// 解密密文
	return c.decrypt(cipherData)
}

// DecryptMsg 处理企微用户发来的 POST 加密消息 XML 体
func (c *WXBizMsgCrypt) DecryptMsg(msgSignature, timestamp, nonce string, postData []byte) (*PlainXMLMsg, error) {
	// 解析外层加密 XML
	var encMsg EncryptedXMLMsg
	if err := xml.Unmarshal(postData, &encMsg); err != nil {
		return nil, fmt.Errorf("xml unmarshal encrypt body error: %w", err)
	}

	// 校验签名
	if !c.verifySignature(c.token, timestamp, nonce, encMsg.Encrypt, msgSignature) {
		return nil, fmt.Errorf("signature verification failed")
	}

	// Base64 解码密文
	cipherData, err := base64.StdEncoding.DecodeString(encMsg.Encrypt)
	if err != nil {
		return nil, fmt.Errorf("base64 decode msg error: %w", err)
	}

	// 解密得到明文 XML 字节
	plainMsgBytes, err := c.decrypt(cipherData)
	if err != nil {
		return nil, fmt.Errorf("decrypt msg error: %w", err)
	}

	// 解析明文 XML 到结构体
	var plainMsg PlainXMLMsg
	if err := xml.Unmarshal(plainMsgBytes, &plainMsg); err != nil {
		return nil, fmt.Errorf("xml unmarshal plain msg error: %w", err)
	}

	return &plainMsg, nil
}
