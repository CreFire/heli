package xtoken

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"game/deps/xlog"
	"game/deps/xtime"
	"math/rand/v2"
	"strconv"
	"strings"
)

type TokenType int32

const (
	CLIENT_TOKEN TokenType = 0
	SERVER_TOKEN TokenType = 1
)

var DefaultCoder = NewTokenCoder("se&42@12", 7*24*60*60)

func ServerTokenEncode(sender, receiver string) (string, error) {
	return DefaultCoder.InternalTokenEncode(sender, receiver)
}

func ServerTokenDecode(token string, receiver string) error {
	return DefaultCoder.InternalTokenDecode(token, receiver)
}

func UserTokenEncode(userId int64, machineId string) (string, error) {
	return DefaultCoder.SimpleTokenEncode(userId, machineId)
}

func UserTokenDecode(token string, machineId string) (int64, error) {
	return DefaultCoder.SimpleTokenDecode(token, machineId)
}

type TokenCoder struct {
	Secret string
	Expire int64
}

func (t *TokenCoder) gcmCipher() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(t.Secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (t *TokenCoder) gcmSeal(src []byte) (string, error) {
	aead, err := t.gcmCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := crand.Read(nonce); err != nil {
		return "", err
	}
	out := aead.Seal(nonce, nonce, src, nil)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(out), nil
}

func (t *TokenCoder) gcmOpen(token string) ([]byte, error) {
	aead, err := t.gcmCipher()
	if err != nil {
		return nil, err
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(token)
	if err != nil {
		return nil, err
	}
	nonceSize := aead.NonceSize()
	if len(raw) <= nonceSize {
		return nil, errors.New("token decode error")
	}
	nonce := raw[:nonceSize]
	ciphertext := raw[nonceSize:]
	return aead.Open(nil, nonce, ciphertext, nil)
}

func (t *TokenCoder) InternalTokenEncode(sender, receiver string) (string, error) {
	src := fmt.Sprintf("%s:%s:%d", sender, receiver, xtime.NowUnix()+xtime.HourSec)
	return t.gcmSeal([]byte(src))
}

func (t *TokenCoder) InternalTokenDecode(token string, receiver string) error {
	plain, err := t.gcmOpen(token)
	if err != nil {
		xlog.Warnf("token decode, gcm open failed, %s", err.Error())
		return err
	}
	tmp := strings.Split(string(plain), ":")
	if len(tmp) != 3 {
		xlog.Warnf("token decode, split failed, %d", len(tmp))
		return errors.New("token decode error")
	}

	if tmp[1] != receiver {
		return errors.New("token decode error, receiver not match")
	}

	// 检查过期时间
	expire, err := strconv.ParseInt(tmp[2], 10, 64)
	if err != nil {
		return errors.New("token decode error, invalid expire time")
	}

	if expire < xtime.NowUnix() {
		return errors.New("token expired")
	}

	return nil
}

func (t *TokenCoder) GetSecret() string {
	return t.Secret
}

func (t *TokenCoder) SimpleTokenEncode(UserId int64, MachineCode string) (string, error) {
	if len(MachineCode) <= 7 {
		return "", errors.New("machine code len less than 8")
	}
	start := rand.IntN(len(MachineCode) - 7)
	data := MachineCode[start : start+7]
	rnd := rand.Int32N(100000)
	src := fmt.Sprintf("%d:%s:%d:%d", rnd, data, xtime.NowUnix()+xtime.DaySec*7, UserId)
	return t.gcmSeal([]byte(src))
}

func (t *TokenCoder) SimpleTokenDecode(token string, MachineCode string) (int64, error) {
	plain, err := t.gcmOpen(token)
	if err != nil {
		xlog.Warnf("token decode, gcm open failed, %s", err.Error())
		return 0, err
	}
	tmp := strings.Split(string(plain), ":")
	if len(tmp) != 4 {
		xlog.Warnf("token decode, split failed, %d", len(tmp))
		return 0, errors.New("decode error data len")
	}
	userId, err := strconv.ParseInt(tmp[3], 10, 64)
	if err != nil {
		xlog.Warnf("token decode, strconv failed, %s", tmp[3])
		return 0, errors.New("decode error parse userid")
	}
	expire, err := strconv.ParseInt(tmp[2], 10, 64)
	if err != nil {
		xlog.Warnf("token decode, strconv failed, %s", tmp[2])
		return 0, errors.New("decode error parse expire")
	}

	if expire < xtime.NowUnix() {
		xlog.Warnf("token decode, expire failed, %d", expire)
		return 0, errors.New("token expired")
	}

	if len(MachineCode) > 0 && !strings.Contains(MachineCode, tmp[1]) {
		xlog.Warnf("token decode, machine code failed, %s not in machine code: %s", tmp[1], MachineCode)
		return 0, errors.New("machine change")
	}

	return userId, nil
}

// NewTokenCoder 创建新的TokenCoder实例
func NewTokenCoder(secret string, expire int64) *TokenCoder {
	return &TokenCoder{
		Secret: secret,
		Expire: expire,
	}
}
