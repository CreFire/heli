package basal

import (
	"crypto/cipher"
	"crypto/des"
	"fmt"
	"game/deps/xlog"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"time"
)

func IsKTimesPowerOfTwo(n, k int) bool {
	if n%k != 0 {
		return false
	}
	m := n / k
	if m < 1 {
		return false
	}
	return (m & (m - 1)) == 0
}
func NextPrime(n int32) int32 {
	if next, ok := nextPrimeLookup[n]; ok {
		return next
	}
	return 0
}

var nextPrimeLookup = map[int32]int32{
	1:     2,
	2:     3,
	4:     5,
	8:     11,
	16:    17,
	64:    67,
	128:   131,
	256:   257,
	512:   521,
	1024:  1031,
	4096:  4099,
	8192:  8209,
	16384: 16411,
}

func SafeRun(f func()) (retErr error) {
	defer func() {
		if err := recover(); err != nil {
			xlog.Errorf("safe run error %v, stack info: %v", err, string(debug.Stack()))
			retErr = fmt.Errorf("panic recover cause: %v", err)
		}
	}()

	f()

	return nil
}

func SafeGo(f func()) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				xlog.Errorf("safe run error %v, stack info: %v", err, string(debug.Stack()))
			}
		}()

		f()
	}()
}

func SimpleHash(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// SimpleStrHash 使用 FNV-1a 算法计算字符串的哈希值
func SimpleStrHash(key string) uint64 {
	const (
		offset uint64 = 14695981039346656037
		prime  uint64 = 1099511628211
	)

	var hash = offset
	for i := 0; i < len(key); i++ {
		hash ^= uint64(key[i])
		hash *= prime
	}
	return hash
}

// EncTogDES DES加密函数
func EncTogDES(src, key, iv []byte) []byte {
	block, err := des.NewCipher(key)
	if err != nil {
		return nil
	}

	// 填充数据到8字节的倍数
	src = pkcs5Padding(src, block.BlockSize())

	mode := cipher.NewCBCEncrypter(block, iv)
	dst := make([]byte, len(src))
	mode.CryptBlocks(dst, src)

	return dst
}

// DecTogDES DES解密函数
func DecTogDES(src, key, iv []byte) []byte {
	block, err := des.NewCipher(key)
	if err != nil {
		return nil
	}

	if len(src)%block.BlockSize() != 0 {
		return nil
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	dst := make([]byte, len(src))
	mode.CryptBlocks(dst, src)

	// 去除填充
	return pkcs5UnPadding(dst)
}

// PKCS5填充
func pkcs5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := make([]byte, padding)
	for i := range padtext {
		padtext[i] = byte(padding)
	}
	return append(ciphertext, padtext...)
}

// PKCS5去填充
func pkcs5UnPadding(origData []byte) []byte {
	length := len(origData)
	if length == 0 {
		return nil
	}
	unpadding := int(origData[length-1])
	if unpadding > length {
		return nil
	}
	return origData[:(length - unpadding)]
}

func WriteMemProfile() {
	now := time.Now()
	filePaths := strings.Split(os.Args[0], "/")

	memF, err := os.Create(fmt.Sprintf("%d%02d%02d-%02d-%02d-%02d_%v_mem_profile", now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second(), filePaths[len(filePaths)-1]))
	if err != nil {
		xlog.Errorf(err.Error())
		return
	}

	defer func() {
		err = memF.Close()
		if err != nil {
			xlog.Errorf(err.Error())
		}
	}()

	if err = pprof.WriteHeapProfile(memF); err != nil {
		xlog.Errorf(err.Error())
	}
}

const (
	SleepDuration = 20 * time.Second
)

func WriteCPUProfile() {
	xlog.Infof("SignalCpuProfile Signal")
	go func() {
		now := time.Now()

		filePaths := strings.Split(os.Args[0], "/")

		cpuF, err := os.Create(fmt.Sprintf("%d%02d%02d-%02d-%02d-%02d_%v_cpu_profile",
			now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), filePaths[len(filePaths)-1]))
		if err != nil {
			xlog.Errorf(err.Error())
			return
		}
		_ = pprof.StartCPUProfile(cpuF)
		defer pprof.StopCPUProfile()

		time.Sleep(SleepDuration)
	}()
}

func Min[V int | int32 | uint32 | int64 | uint64](a, b V) V {
	if a < b {
		return a
	}
	return b
}

func Max[V int | int32 | uint32 | int64 | uint64](a, b V) V {
	if a > b {
		return a
	}
	return b
}
