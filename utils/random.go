package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	mathRand "math/rand"
	"time"
)

func init() {
	mathRand.Seed(time.Now().UnixNano())
}

func intPow(base, exp int) int {
	result := 1
	for exp > 0 {
		result *= base
		exp--
	}
	return result
}

// GenerateRandomCode 随机数种子生成定长验证码，安全性不如crypto/rand生成的密码学安全验证码，但性能略高，属于数学伪随机。
func GenerateRandomCode(length int) string {
	if length <= 0 {
		length = 6
	}
	return fmt.Sprintf("%0*d", length, mathRand.Intn(intPow(10, length)))
}

// GenerateSecureRandomCode 密码学安全的随机数生成定长验证码，安全性高于数学伪随机。
func GenerateSecureRandomCode(length int) (string, error) {
	if length <= 0 {
		length = 6
	}

	max := big.NewInt(int64(intPow(10, length)))

	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", length, n), nil
}
