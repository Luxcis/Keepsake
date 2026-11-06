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

// Options 是 Client 的构造参数。
type Options struct {
	SecretID  string
	SecretKey string
	Region    string
	Bucket    string
	// Origin 是可选的自定义源站域名（COS 控制台「自定义源站域名」）。
	// 支持 example.com 或 https://example.com（默认 https，端口可保留）。
	// 为空时使用 https://{bucket}.cos.{region}.myqcloud.com。
	// 预签名会按该域名计算 Host，因此域名必须已 CNAME 到该桶且 COS 能按此
	// Host 校验签名；CDN 加速域名与带路径前缀的域名不适用。
	Origin     string
	URLExpires time.Duration
}

// New 根据配置构建 SDK 客户端；region 形如 ap-guangzhou。
// 自定义源站域名非法时返回错误（启动即失败，避免上线后才发现签名 403）。
func New(opts Options) (*Client, error) {
	bucketURL, err := bucketURL(opts.Bucket, opts.Region, opts.Origin)
	if err != nil {
		return nil, err
	}
	b := &cos.BaseURL{BucketURL: bucketURL}
	core := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{SecretID: opts.SecretID, SecretKey: opts.SecretKey},
	})
	return &Client{core: core, id: opts.SecretID, key: opts.SecretKey, expires: opts.URLExpires}, nil
}

// bucketURL 生成 BucketURL：优先使用自定义源站域名，否则回落到 COS 默认域名。
func bucketURL(bucket, region, origin string) (*url.URL, error) {
	if origin == "" {
		u, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", bucket, region))
		if err != nil {
			return nil, fmt.Errorf("parse cos bucket url (bucket=%q region=%q): %w", bucket, region, err)
		}
		return u, nil
	}
	u, err := parseOrigin(origin)
	if err != nil {
		return nil, fmt.Errorf("invalid COS_ORIGIN_DOMAIN %q: %w", origin, err)
	}
	return u, nil
}

// parseOrigin 规范化自定义源站域名：缺省补 https、去掉末尾斜杠，并拒绝
// 带路径/查询/用户信息或非 http(s) 的值（COS 自定义源站域名只接受主机名）。
func parseOrigin(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("scheme must be http or https, got %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return nil, errors.New("host is empty")
	}
	if u.User != nil {
		return nil, errors.New("user info is not allowed")
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("path, query and fragment are not allowed")
	}
	// DNS 主机名不区分大小写，但签名按字符串比较：统一小写，避免浏览器
	// 发送的 Host 与签名 Host 大小写不一致导致 403。
	u.Host = strings.ToLower(u.Host)
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
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
