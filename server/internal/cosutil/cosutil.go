// Package cosutil 封装腾讯 COS 私有桶访问：短时效预签名 PUT、读取 URL（CDN
// 无签名或签名 GET）、对象 HEAD 校验与删除。
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
	// SignGET 生成原图读取 URL：配置 CDN 时为无签名地址，否则为短时效签名 URL。
	SignGET(key string) (string, error)
	// SignGETThumb 生成缩略图 URL：配置 CDN 时无签名，否则把 COS 缩放处理
	// 参数签入 URL。处理参数不能在签名后追加（私有读桶会判 403）。
	SignGETThumb(key string) (string, error)
	// SignGETPreview 生成灯箱显示尺寸 URL（小于原图），签名规则同 SignGETThumb。
	SignGETPreview(key string) (string, error)
	// Head 返回对象大小与 Content-Type；对象不存在返回 ErrObjectMissing。
	Head(key string) (size int64, contentType string, err error)
	// Delete 删除对象；对象不存在视为成功（幂等）。
	Delete(key string) error
}

// Client 基于 COS Go SDK 的 Backend 实现。
type Client struct {
	// core 使用 COS 默认域名：上传签名、HEAD 校验与删除。
	core *cos.Client
	// cdn 是可选 CDN 域名：读取类 URL 不带签名；为空时读取回落到 core
	// 的短时效签名 URL（私有桶直读）。
	cdn *url.URL
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
	// CDNDomain 是可选 CDN 域名（COS_CDN_DOMAIN），如 photos.example.com。
	// 配置后读取类 URL（原图/缩略图/灯箱）直接使用该域名且不带 COS 签名，
	// 因此需由 CDN 侧完成回源鉴权或 Bucket 设为公共读。
	// 上传 PUT、HEAD 校验与删除始终使用 COS 默认域名。
	// 支持 example.com 或 https://example.com（默认 https，端口可保留）；
	// 为空时读取继续使用默认域名的短时效签名 URL。
	CDNDomain  string
	URLExpires time.Duration
}

// New 根据配置构建 SDK 客户端；region 形如 ap-guangzhou。
// CDN 域名非法时返回错误（启动即失败，避免上线后才发现读取地址不可用）。
func New(opts Options) (*Client, error) {
	coreURL, err := defaultBucketURL(opts.Bucket, opts.Region)
	if err != nil {
		return nil, err
	}
	core := newSDKClient(coreURL, opts)
	var cdn *url.URL
	if opts.CDNDomain != "" {
		cdn, err = parseCDNDomain(opts.CDNDomain)
		if err != nil {
			return nil, fmt.Errorf("invalid COS_CDN_DOMAIN %q: %w", opts.CDNDomain, err)
		}
	}
	return &Client{core: core, cdn: cdn, id: opts.SecretID, key: opts.SecretKey, expires: opts.URLExpires}, nil
}

func newSDKClient(bucketURL *url.URL, opts Options) *cos.Client {
	return cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{
		Transport: &cos.AuthorizationTransport{SecretID: opts.SecretID, SecretKey: opts.SecretKey},
	})
}

func defaultBucketURL(bucket, region string) (*url.URL, error) {
	u, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", bucket, region))
	if err != nil {
		return nil, fmt.Errorf("parse cos bucket url (bucket=%q region=%q): %w", bucket, region, err)
	}
	return u, nil
}

// parseCDNDomain 规范化 CDN 域名：缺省补 https、去掉末尾斜杠，并拒绝
// 带路径/查询/用户信息或非 http(s) 的值（读取地址只接受主机名）。
func parseCDNDomain(raw string) (*url.URL, error) {
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
	// DNS 主机名不区分大小写，统一小写便于日志与缓存键一致。
	u.Host = strings.ToLower(u.Host)
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
}

func (c *Client) Enabled() bool { return true }

func (c *Client) SignPUT(key, contentType string) (string, time.Time, error) {
	// 上传始终直连 COS 默认域名：CDN 只承担读取，写操作不依赖 CDN。
	return c.presign(c.core, http.MethodPut, key, contentType, nil)
}

// SignGET 返回原图读取地址；配置 CDN 时为无签名 URL，否则为默认域名的
// 短时效签名 URL。
func (c *Client) SignGET(key string) (string, error) {
	if c.cdn != nil {
		return c.cdnURL(key, ""), nil
	}
	signed, _, err := c.presign(c.core, http.MethodGet, key, "", nil)
	return signed, err
}

// thumbOp：数据万象缩放指令（宽度 1400 等比缩放），整体作为参数 key、值为空参与签名。
const thumbOp = "imageMogr2/thumbnail/1400x"

// previewOp：灯箱显示尺寸（最长边 2400、等比缩小、原图更小则不放大）。
// 原图可达 30MB，直接下发会让灯箱切换长时间停在上一张；缩放到屏幕够用的尺寸后
// 每张通常只有几百 KB。`>` 后缀是 COS 的“仅缩小”语义。
const previewOp = "imageMogr2/thumbnail/2400x2400>"

// SignGETThumb 生成缩略图 URL：配置 CDN 时无签名，否则签入缩放处理参数。
// 私有读桶下处理参数必须在签名时携带；签名后再拼到 URL 上会被 COS 校验
// 判为参数与签名不符（403 SignatureDoesNotMatch），不能在客户端追加。
func (c *Client) SignGETThumb(key string) (string, error) {
	return c.signProcessed(key, thumbOp)
}

// SignGETPreview 生成灯箱显示尺寸 URL：配置 CDN 时无签名，否则签入处理参数。
func (c *Client) SignGETPreview(key string) (string, error) {
	return c.signProcessed(key, previewOp)
}

func (c *Client) signProcessed(key, op string) (string, error) {
	if c.cdn != nil {
		return c.cdnURL(key, op), nil
	}
	q := url.Values{op: {""}}
	signed, _, err := c.presign(c.core, http.MethodGet, key, "", &q)
	return signed, err
}

// cdnURL 拼接无签名读取地址。对象 Key 由服务端生成，这里仍交给 net/url
// 做路径转义，避免特殊字符构造出意外 URL；处理参数按 COS/CDN 约定的字面量
// 斜杠形式下发（imageMogr2/thumbnail/...），仅把 `>` 转义为 %3E。
func (c *Client) cdnURL(key, op string) string {
	u := *c.cdn
	u.Path = "/" + strings.TrimPrefix(key, "/")
	u.RawPath = ""
	u.RawQuery = strings.ReplaceAll(op, ">", "%3E")
	return u.String()
}

func (c *Client) presign(cl *cos.Client, method, objectKey, contentType string, query *url.Values) (string, time.Time, error) {
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

	signed, err := cl.Object.GetPresignedURL(ctx, method, objectKey,
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
