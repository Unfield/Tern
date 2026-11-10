package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// S3Options configure an S3-compatible bucket.
type S3Options struct {
	// Endpoint is the service URL, such as
	// "https://s3.eu-central-1.amazonaws.com", "https://<account>.r2.cloudflarestorage.com"
	// or "http://localhost:9000" for MinIO.
	Endpoint string
	// Region is the bucket's region; "auto" for R2, "us-east-1" for MinIO.
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	// PathStyle addresses the bucket as <endpoint>/<bucket>/<key> instead
	// of <bucket>.<endpoint host>/<key>. MinIO and most self-hosted servers
	// need it.
	PathStyle bool
	// HTTPClient is used for requests; the default has a 5 minute timeout.
	HTTPClient *http.Client
}

// S3Bucket is a bucket on an S3-compatible service. Requests are signed
// with AWS Signature Version 4.
type S3Bucket struct {
	o    S3Options
	base *url.URL
	now  func() time.Time
}

// NewS3 returns an S3 bucket.
func NewS3(o S3Options) (*S3Bucket, error) {
	u, err := url.Parse(strings.TrimRight(o.Endpoint, "/"))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("storage: invalid S3 endpoint %q", o.Endpoint)
	}
	if o.Bucket == "" || o.Region == "" || o.AccessKey == "" || o.SecretKey == "" {
		return nil, errors.New("storage: S3 needs a bucket, region, access key and secret key")
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 5 * time.Minute}
	}
	return &S3Bucket{o: o, base: u, now: time.Now}, nil
}

// objectURL returns the URL of key, or of the bucket when key is "".
func (b *S3Bucket) objectURL(key string, query url.Values) *url.URL {
	u := *b.base
	p := u.Path
	if b.o.PathStyle {
		p += "/" + b.o.Bucket
	} else {
		u.Host = b.o.Bucket + "." + u.Host
	}
	p += "/" + key
	u.Path = p
	u.RawPath = uriEncode(p, false)
	if query != nil {
		u.RawQuery = canonicalQuery(query)
	}
	return &u
}

// uriEncode encodes like AWS's UriEncode: everything but unreserved
// characters, and "/" unless encodeSlash.
func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var parts []string
	for _, k := range keys {
		vals := slices.Clone(q[k])
		slices.Sort(vals)
		for _, v := range vals {
			parts = append(parts, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}
	return strings.Join(parts, "&")
}

const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
const unsignedPayload = "UNSIGNED-PAYLOAD"

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func (b *S3Bucket) signingKey(date string) []byte {
	k := hmacSHA256([]byte("AWS4"+b.o.SecretKey), date)
	k = hmacSHA256(k, b.o.Region)
	k = hmacSHA256(k, "s3")
	return hmacSHA256(k, "aws4_request")
}

// sign adds the Authorization header to req (SigV4, header-based).
func (b *S3Bucket) sign(req *http.Request, payloadHash string, t time.Time) {
	amzDate := t.UTC().Format("20060102T150405Z")
	date := amzDate[:8]
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	host := req.URL.Host
	headers := map[string]string{"host": host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if lk == "x-amz-date" || lk == "x-amz-content-sha256" || lk == "range" || lk == "content-type" || lk == "content-md5" || strings.HasPrefix(lk, "x-amz-") {
			headers[lk] = strings.Join(strings.Fields(strings.Join(v, ",")), " ")
		}
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	slices.Sort(names)
	var canonHeaders strings.Builder
	for _, n := range names {
		canonHeaders.WriteString(n + ":" + headers[n] + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{req.Method, uriPath(req.URL), req.URL.RawQuery, canonHeaders.String(), signed, payloadHash}, "\n")
	scope := date + "/" + b.o.Region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	sig := hex.EncodeToString(hmacSHA256(b.signingKey(date), toSign))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", b.o.AccessKey, scope, signed, sig))
}

func uriPath(u *url.URL) string {
	if u.RawPath != "" {
		return u.RawPath
	}
	if u.Path == "" {
		return "/"
	}
	return uriEncode(u.Path, false)
}

// presign returns a URL with the signature in the query (SigV4 presigned).
func (b *S3Bucket) presign(method, key string, ttl time.Duration, t time.Time) string {
	if ttl <= 0 || ttl > 7*24*time.Hour {
		ttl = 7 * 24 * time.Hour
	}
	amzDate := t.UTC().Format("20060102T150405Z")
	date := amzDate[:8]
	scope := date + "/" + b.o.Region + "/s3/aws4_request"
	q := url.Values{
		"X-Amz-Algorithm":     {"AWS4-HMAC-SHA256"},
		"X-Amz-Credential":    {b.o.AccessKey + "/" + scope},
		"X-Amz-Date":          {amzDate},
		"X-Amz-Expires":       {strconv.Itoa(int(ttl.Seconds()))},
		"X-Amz-SignedHeaders": {"host"},
	}
	u := b.objectURL(key, q)
	canonical := strings.Join([]string{method, uriPath(u), u.RawQuery, "host:" + u.Host + "\n", "host", unsignedPayload}, "\n")
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	sig := hex.EncodeToString(hmacSHA256(b.signingKey(date), toSign))
	u.RawQuery += "&X-Amz-Signature=" + sig
	return u.String()
}

// PresignGet implements Presigner: a URL that downloads key until ttl
// passes (at most 7 days).
func (b *S3Bucket) PresignGet(key string, ttl time.Duration) (string, error) {
	if err := ValidKey(key); err != nil {
		return "", err
	}
	return b.presign(http.MethodGet, key, ttl, b.now()), nil
}

// PresignPut returns a URL that browsers can PUT a file to directly, so
// large uploads don't pass through the app.
func (b *S3Bucket) PresignPut(key string, ttl time.Duration) (string, error) {
	if err := ValidKey(key); err != nil {
		return "", err
	}
	return b.presign(http.MethodPut, key, ttl, b.now()), nil
}

type s3Error struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

func (b *S3Bucket) do(ctx context.Context, method, key string, query url.Values, body io.Reader, size int64, header http.Header) (*http.Response, error) {
	if key != "" {
		if err := ValidKey(key); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, b.objectURL(key, query).String(), body)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	payload := emptyHash
	if body != nil {
		payload = unsignedPayload
		if size >= 0 {
			req.ContentLength = size
		}
	}
	b.sign(req, payload, b.now())
	res, err := b.o.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage: S3 %s %s: %w", method, key, err)
	}
	if res.StatusCode == http.StatusNotFound {
		_ = res.Body.Close()
		return nil, ErrNotFound
	}
	if res.StatusCode >= 300 {
		defer func() { _ = res.Body.Close() }()
		var e s3Error
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
		_ = xml.Unmarshal(raw, &e)
		if e.Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage: S3 %s %s: HTTP %d %s: %s", method, key, res.StatusCode, e.Code, e.Message)
	}
	return res, nil
}

// Put implements Bucket.
func (b *S3Bucket) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	h := http.Header{}
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	if size < 0 {
		// S3 needs the length up front; buffer bodies of unknown size.
		raw, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		r, size = strings.NewReader(string(raw)), int64(len(raw))
	}
	res, err := b.do(ctx, http.MethodPut, key, nil, r, size, h)
	if err != nil {
		return err
	}
	return res.Body.Close()
}

func objectFrom(key string, h http.Header) Object {
	size, _ := strconv.ParseInt(h.Get("Content-Length"), 10, 64)
	mod, _ := http.ParseTime(h.Get("Last-Modified"))
	ct := h.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	return Object{Key: key, Size: size, ContentType: ct, Modified: mod}
}

// Get implements Bucket.
func (b *S3Bucket) Get(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	res, err := b.do(ctx, http.MethodGet, key, nil, nil, 0, nil)
	if err != nil {
		return nil, Object{}, err
	}
	return res.Body, objectFrom(key, res.Header), nil
}

// Stat implements Bucket.
func (b *S3Bucket) Stat(ctx context.Context, key string) (Object, error) {
	res, err := b.do(ctx, http.MethodHead, key, nil, nil, 0, nil)
	if err != nil {
		return Object{}, err
	}
	_ = res.Body.Close()
	return objectFrom(key, res.Header), nil
}

// Delete implements Bucket.
func (b *S3Bucket) Delete(ctx context.Context, key string) error {
	res, err := b.do(ctx, http.MethodDelete, key, nil, nil, 0, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return res.Body.Close()
}

// List implements Bucket.
func (b *S3Bucket) List(ctx context.Context, prefix string) ([]Object, error) {
	out := []Object{}
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		res, err := b.do(ctx, http.MethodGet, "", q, nil, 0, nil)
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("storage: the bucket %s does not exist", b.o.Bucket)
		}
		if err != nil {
			return nil, err
		}
		var page struct {
			Contents []struct {
				Key          string    `xml:"Key"`
				Size         int64     `xml:"Size"`
				LastModified time.Time `xml:"LastModified"`
			} `xml:"Contents"`
			IsTruncated           bool   `xml:"IsTruncated"`
			NextContinuationToken string `xml:"NextContinuationToken"`
		}
		err = xml.NewDecoder(res.Body).Decode(&page)
		_ = res.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("storage: listing: %w", err)
		}
		for _, c := range page.Contents {
			out = append(out, Object{Key: c.Key, Size: c.Size, Modified: c.LastModified})
		}
		if !page.IsTruncated || page.NextContinuationToken == "" {
			return out, nil
		}
		token = page.NextContinuationToken
	}
}
