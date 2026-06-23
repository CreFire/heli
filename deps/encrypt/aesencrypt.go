package encrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"io"
)

const gcmHeader = "GCM1"

// AesEncodeData uses AES-GCM with a shared secret.
// Output format: "GCM1" + Nonce + EncryptedData (ciphertext includes tag).
func AesEncodeData(data []byte, sharedSecret []byte) ([]byte, error) {
	// 1. 检查共享密钥是否有效
	if len(sharedSecret) == 0 {
		return nil, errors.New("AES 加密需要一个共享密钥")
	}

	// 2. 创建 AES 加密块
	block, err := aes.NewCipher(sharedSecret)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	sealed := aead.Seal(nil, nonce, data, []byte(gcmHeader))
	out := make([]byte, 0, len(gcmHeader)+len(nonce)+len(sealed))
	out = append(out, gcmHeader...)
	out = append(out, nonce...)
	out = append(out, sealed...)

	return out, nil
}

// AesDecodeData uses AES-GCM with a shared secret.
// Input format: "GCM1" + Nonce + EncryptedData (ciphertext includes tag).
// Legacy CBC (IV + EncryptedData) is still accepted for backward compatibility.
func AesDecodeData(data []byte, sharedSecret []byte) ([]byte, error) {
	// 1. 检查共享密钥是否有效
	if len(sharedSecret) == 0 {
		return nil, errors.New("AES 解密需要一个共享密钥")
	}

	// 2. 创建 AES 加密块
	block, err := aes.NewCipher(sharedSecret)
	if err != nil {
		return nil, err
	}

	if bytes.HasPrefix(data, []byte(gcmHeader)) {
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		headerLen := len(gcmHeader)
		nonceSize := aead.NonceSize()
		if len(data) < headerLen+nonceSize+aead.Overhead() {
			return nil, errors.New("gcm: ciphertext too short")
		}
		nonce := data[headerLen : headerLen+nonceSize]
		ciphertext := data[headerLen+nonceSize:]
		plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(gcmHeader))
		if err != nil {
			return nil, err
		}
		return plaintext, nil
	}

	blockSize := block.BlockSize()

	// 3. 检查密文长度是否有效
	if len(data) < blockSize {
		return nil, errors.New("密文长度过短")
	}

	// 4. 从数据开头提取 IV
	iv := data[:blockSize]
	ciphertext := data[blockSize:]

	// 5. 检查密文是否为块大小的整数倍
	if len(ciphertext)%blockSize != 0 {
		return nil, errors.New("密文不是块大小的整数倍")
	}

	// 6. 创建一个 CBC 模式的解密器
	mode := cipher.NewCBCDecrypter(block, iv)

	// 7. 执行解密
	// Go 的解密是 in-place 的，所以我们可以直接在 ciphertext 上操作
	mode.CryptBlocks(ciphertext, ciphertext)

	// 8. 移除 PKCS7 填充
	return pkcs7Unpad(ciphertext)
}

// pkcs7Pad 对数据进行 PKCS7 填充
// C# 的 CryptoStream 默认使用这种填充方式
func pkcs7Pad(data []byte, blockSize int) []byte {
	// 计算需要填充的长度
	padding := blockSize - len(data)%blockSize
	// 创建一个字节切片，内容是填充的长度值
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	// 将填充内容附加到原始数据末尾
	return append(data, padText...)
}

// pkcs7Unpad 移除 PKCS7 填充
func pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("pkcs7: data is empty")
	}
	// 获取最后一个字节，它代表填充的长度
	unpadding := int(data[length-1])
	// 检查填充长度是否有效
	if unpadding == 0 || unpadding > length || unpadding > aes.BlockSize {
		return nil, errors.New("pkcs7: invalid padding")
	}
	var invalid byte
	for _, b := range data[length-unpadding:] {
		invalid |= b ^ byte(unpadding)
	}
	if subtle.ConstantTimeByteEq(invalid, 0) != 1 {
		return nil, errors.New("pkcs7: invalid padding")
	}
	// 返回移除填充后的数据
	return data[:(length - unpadding)], nil
}
