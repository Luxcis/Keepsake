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

func (NoopBackend) SignGETThumb(string) (string, error) { return "", ErrNotConfigured }

func (NoopBackend) SignGETPreview(string) (string, error) { return "", ErrNotConfigured }

func (NoopBackend) Head(string) (int64, string, error) { return 0, "", ErrNotConfigured }

func (NoopBackend) Delete(string) error { return ErrNotConfigured }

// Backend 是上传链路依赖的最小集合（可注入 stub 供测试）。
type Backend interface {
	Enabled() bool
	// SignPUT 生成受限 PUT 预签名 URL（Content-Type 一并参与签名）。
	SignPUT(key, contentType string) (uploadURL string, expireAt time.Time, err error)
	// SignGET 生成短时效读取 URL。
	SignGET(key string) (string, error)
	// SignGETThumb 生成签入 COS 缩放处理参数的缩略图 URL。
	// 处理参数必须参与签名：私有读桶下签名后拼参数会被校验拒绝（403）。
	SignGETThumb(key string) (string, error)
	// SignGETPreview 生成签入显示尺寸缩放参数的 URL（灯箱用，小于原图）。
	// 与 SignGETThumb 同一签名约束：处理参数必须参与签名。
	SignGETPreview(key string) (string, error)
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
	return c.presign(http.MethodPut, key, contentType, nil)
}

func (c *Client) SignGET(key string) (string, error) {
	signed, _, err := c.presign(http.MethodGet, key, "", nil)
	return signed, err
}

// thumbOp：数据万象缩放指令（宽度 1400 等比缩放），整体作为参数 key、值为空参与签名。
const thumbOp = "imageMogr2/thumbnail/1400x"

// previewOp：灯箱显示尺寸（最长边 2400、等比缩小、原图更小则不放大）。
// 原图可达 30MB，直接下发会让灯箱切换长时间停在上一张；缩放到屏幕够用的尺寸后
// 每张通常只有几百 KB。`>` 后缀是 COS 的“仅缩小”语义。
const previewOp = "imageMogr2/thumbnail/2400x2400>"

// SignGETThumb 生成签入缩放处理参数的短时效缩略图 URL。
// 私有读桶下处理参数必须在签名时携带；签名后再拼到 URL 上会被 COS 校验
// 判为参数与签名不符（403 SignatureDoesNotMatch），不能在客户端追加。
func (c *Client) SignGETThumb(key string) (string, error) {
	return c.signProcessed(key, thumbOp)
}

// SignGETPreview 生成签入显示尺寸缩放参数的短时效 URL（灯箱用）。
func (c *Client) SignGETPreview(key string) (string, error) {
	return c.signProcessed(key, previewOp)
}

func (c *Client) signProcessed(key, op string) (string, error) {
	q := url.Values{op: {""}}
	signed, _, err := c.presign(http.MethodGet, key, "", &q)
	return signed, err
}

func (c *Client) presign(method, objectKey, contentType string, query *url.Values) (string, time.Time, error) {
	expireAt := time.Now().Add(c.expires)
	opt := &cos.PresignedURLOptions{}
	if contentType != "" {
		// COS 预签名约定：Content-Type 参与签名，浏览器直传时必须携带同一值，
		// 否则 403 —— 这也是确认阶段 HEAD 校验的依据。
		h := http.Header{"Content-Type": []string{contentType}}
		opt.Header = &h
	}
	if query != nil {
		opt.Query = query
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
