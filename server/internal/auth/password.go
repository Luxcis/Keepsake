// Package auth 实现计划 §5.1 的安全基座：
// Argon2id 密码哈希、双会话（站点门禁 / 管理员）、登录限速、失败关闭策略。
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// 参数按 OWASP Password Storage 建议值（2024+）：m=19456(19MiB), t=2, p=1。
const (
	argonMemory  = 19456
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

const hashPrefix = "argon2id"

var ErrMismatch = errors.New("password mismatch")

// HashPassword 生成 Argon2id 哈希（PHC 字符串格式），便于未来改参后的兼容校验：
// argon2id$v=19$m=19456,t=2,p=1$<b64salt>$<b64hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		hashPrefix, argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword 按 PHC 自述的参数重算并常量时间比对。参数允许与当前推荐不同（旧哈希仍可校验）。
// PHC 格式共 5 段：argon2id$v=19$m,t,p$salt$hash。
func VerifyPassword(password, phc string) error {
	parts := strings.Split(phc, "$")
	if len(parts) != 5 || parts[0] != hashPrefix {
		return errors.New("unsupported hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[1], "v=%d", &version); err != nil {
		return fmt.Errorf("parse version: %w", err)
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return fmt.Errorf("parse params: %w", err)
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[3])
	if err != nil {
		return fmt.Errorf("decode salt: %w", err)
	}
	want, err := b64.DecodeString(parts[4])
	if err != nil || len(want) == 0 {
		return fmt.Errorf("decode hash: %w", err)
	}

	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) == 1 {
		return nil
	}
	return ErrMismatch
}
