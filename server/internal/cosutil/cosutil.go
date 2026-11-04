// Package cosutil 封装腾讯 COS 私有桶访问：短时效预签名 PUT/GET、对象 HEAD 校验与删除。
// 计划 §4.3/§5.1：对象 Key 由服务端生成；密钥只在此层出现；预签名 URL 限制方法与有效期。
package cosutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

// ErrNotConfigured：环境缺少 COS 密钥/区域/桶名，调用方应返回明确的 503 config_missing。
var ErrNotConfigured = errors.New("cos not configured")

// ErrObjectMissing：HEAD 确认阶段发现对象不存在（浏览器直传失败或 Key 无效）。
var ErrObjectMissing = errors.New("cos object missing")

// NoopBackend 未配置 COS 时的安全占位：所有操作报 ErrNotConfigured。
type NoopBackend struct{}

func (NoopBackend) Enabled() bool { return false }

func (NoopBackend) SignPUT(_, _ string) (string, time.Time, error) {
	return "", time.Time{}, ErrNotConfigured
}

func (NoopBackend) SignGET(string) (string, error) { return "", ErrNotConfigured }

func (NoopBackend) Head(string) (int64, string, error) { return 0, "", ErrNotConfigured }

func (NoopBackend) Delete(string) error { return ErrNotConfigured }

// Backend 是上传链路依赖的最小集合（可注入 stub 供测试）。
type Backend interface {
	Enabled() bool
	// SignPUT 生成受限 PUT 预签名 URL（Content-Type 一并参与签名）。
	SignPUT(key, contentType string) (uploadURL string, expireAt time.Time, err error)
	// SignGET 生成短时效读取 URL（查询参数形式可追加 COS 图片处理指令）。
	SignGET(key string) (string, error)
	// Head 返回对象大小与 Content-Type；对象不存在返回 ErrObjectMissing。
	Head(key string) (size int64, contentType string, err error)
	// Delete 删除对象；对象不存在视为成功（幂等）。
	Delete(key string) error
}

// Client 基于 COS Go SDK 的 Backend 实现。
type Client struct {
	core *cos.Client
	// id/key 只用于预签名计算，不出现在日志或响应（计划 §5.1）。
	id, key string
	expires time.Duration
}

// New 根据环境配置构建 SDK 客户端；region 形如 ap-guangzhou。
func New(secretID, secretKey, region, bucket string, urlExpires time.Duration) *Client {
	u, _ := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", bucket, region))
	b := &cos.BaseURL{BucketURL: u}
	core := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{SecretID: secretID, SecretKey: secretKey},
	})
	return &Client{core: core, id: secretID, key: secretKey, expires: urlExpires}
}

func (c *Client) Enabled() bool { return true }

func (c *Client) SignPUT(key, contentType string) (string, time.Time, error) {
	return c.presign(http.MethodPut, key, contentType)
}

func (c *Client) SignGET(key string) (string, error) {
	signed, _, err := c.presign(http.MethodGet, key, "")
	return signed, err
}

func (c *Client) presign(method, objectKey, contentType string) (string, time.Time, error) {
	expireAt := time.Now().Add(c.expires)
	opt := &cos.PresignedURLOptions{}
	if contentType != "" {
		// COS 预签名约定：Content-Type 参与签名，浏览器直传时必须携带同一值，
		// 否则 403 —— 这也是确认阶段 HEAD 校验的依据。
		h := http.Header{"Content-Type": []string{contentType}}
		opt.Header = &h
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	signed, err := c.core.Object.GetPresignedURL(ctx, method, objectKey,
		c.id, c.key, time.Until(expireAt), opt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("presign %s %s: %w", method, objectKey, err)
	}
	signedURL := signed.String()
	return signedURL, expireAt, nil
}

func (c *Client) Head(objectKey string) (int64, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := c.core.Object.Head(ctx, objectKey, nil)
	if err != nil {
		if isNotFound(err) {
			return 0, "", ErrObjectMissing
		}
		return 0, "", fmt.Errorf("cos head %s: %w", objectKey, err)
	}
	defer resp.Body.Close()
	return resp.ContentLength, resp.Header.Get("Content-Type"), nil
}

func (c *Client) Delete(objectKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := c.core.Object.Delete(ctx, objectKey)
	if err != nil && isNotFound(err) {
		return nil // 幂等：对象不存在即目标状态
	}
	return err
}

// isNotFound 覆盖 SDK 对 404 的几种错误表述。
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "404") || strings.Contains(msg, "NoSuchKey") ||
		strings.Contains(msg, "KeyNotFound")
}
