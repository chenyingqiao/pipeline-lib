package util

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
)

const (
	PasswordForLogs = "n6DDDsd@dXnoKvPm"
)

//EncryptAESCBC aes cbc模式加密
func EncryptAESCBC(origData, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	origData = PKCS7Padding(origData, blockSize)
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	crypted := make([]byte, len(origData))
	blockMode.CryptBlocks(crypted, origData)
	return crypted, nil
}

//DecryptAESCBS aes cbc模式解密
func DecryptAESCBS(crypted, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)
	origData = PKCS7UnPadding(origData)
	return origData, nil
}

//PKCS7Padding 填充
func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

//PKCS7UnPadding 反填充
func PKCS7UnPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

// EncryptPassword 加密密码
func EncryptPassword(pwd string, key string) (string, error) {
	passwordByte, err := EncryptAESCBC([]byte(pwd), []byte(key))
	if err != nil {
		return "", err
	}

	pwdEncrypt := base64.StdEncoding.EncodeToString(passwordByte)

	return pwdEncrypt, nil
}

// DecryptPassword 解密密码
func DecryptPassword(cryptPwd string, key string) (string, error) {
	pwdByte, err := base64.StdEncoding.DecodeString(cryptPwd)
	if err != nil {
		return "", err
	}

	pwd, err := DecryptAESCBS(pwdByte, []byte(key))
	if err != nil {
		return "", err
	}
	password := string(pwd)

	return password, nil
}
