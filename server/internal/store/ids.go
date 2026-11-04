package store

import (
	"crypto/rand"
	"encoding/hex"
)

// newID 生成 UUIDv4 文本主键（计划 §4.1：TEXT UUID/ULID 主键）。
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return formatUUID(b)
}

// NewUUID 导出给 handlers 层生成对象 Key 使用。
func NewUUID() string { return newID() }

func formatUUID(b [16]byte) string {
	h := make([]byte, 32)
	hex.Encode(h[0:8], b[0:4])
	hex.Encode(h[8:12], b[4:6])
	hex.Encode(h[12:16], b[6:8])
	hex.Encode(h[16:20], b[8:10])
	hex.Encode(h[20:32], b[10:16])
	s := string(h)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}
