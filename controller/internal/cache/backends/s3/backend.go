// Package s3 implements an immutable, content-addressed cache on Amazon S3.
package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

var (
	ErrInvalidConfig    = errors.New("invalid S3 cache configuration")
	ErrInvalidKey       = errors.New("invalid content-addressed cache key")
	ErrNotFound         = errors.New("cache object not found")
	ErrImmutable        = errors.New("cache object already exists")
	ErrObjectTooLarge   = errors.New("cache object exceeds configured size limit")
	ErrChecksumMismatch = errors.New("cache object checksum mismatch")
)

const (
	defaultOperationTimeout = 30 * time.Second
	defaultMaxObjectSize    = int64(512 << 20)
	sha256HexLength         = 64
)

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// API is the subset of the AWS S3 client used by Backend. *s3.Client satisfies
// this interface, while tests can provide a deterministic implementation.
type API interface {
	GetObject(context.Context, *awss3.GetObjectInput, ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
	PutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
	DeleteObject(context.Context, *awss3.DeleteObjectInput, ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error)
}

// Config describes the bucket namespace and safety limits for a cache.
type Config struct {
	Bucket               string
	Prefix               string
	OperationTimeout     time.Duration
	MaxObjectSize        int64
	ServerSideEncryption string
	SSEKMSKeyID          string
}

func (c Config) validate() error {
	if !bucketPattern.MatchString(c.Bucket) || strings.Contains(c.Bucket, "..") || net.ParseIP(c.Bucket) != nil {
		return fmt.Errorf("%w: bucket must be a valid lowercase S3 bucket name", ErrInvalidConfig)
	}
	if err := validatePrefix(c.Prefix); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if c.OperationTimeout < 0 {
		return fmt.Errorf("%w: operation timeout cannot be negative", ErrInvalidConfig)
	}
	if c.MaxObjectSize < 0 {
		return fmt.Errorf("%w: maximum object size cannot be negative", ErrInvalidConfig)
	}
	if c.SSEKMSKeyID != "" && c.ServerSideEncryption != "aws:kms" {
		return fmt.Errorf("%w: SSE KMS key requires aws:kms encryption", ErrInvalidConfig)
	}
	if c.ServerSideEncryption != "" && c.ServerSideEncryption != "AES256" && c.ServerSideEncryption != "aws:kms" {
		return fmt.Errorf("%w: unsupported server-side encryption %q", ErrInvalidConfig, c.ServerSideEncryption)
	}
	return nil
}

func (c *Config) defaults() {
	if c.OperationTimeout == 0 {
		c.OperationTimeout = defaultOperationTimeout
	}
	if c.MaxObjectSize == 0 {
		c.MaxObjectSize = defaultMaxObjectSize
	}
}

// Object is the verified result of a cache GET.
type Object struct {
	Key            string
	Content        []byte
	Size           int64
	ContentType    string
	ChecksumSHA256 string
	ETag           string
}

// Backend is safe for concurrent use. S3 supplies durability and the
// If-None-Match condition supplies the immutable-write guarantee.
type Backend struct {
	client API
	config Config
}

func New(client API, config Config) (*Backend, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: S3 client is required", ErrInvalidConfig)
	}
	config.defaults()
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Backend{client: client, config: config}, nil
}

func (b *Backend) operation(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	operationCtx, cancel := context.WithTimeout(ctx, b.config.OperationTimeout)
	return operationCtx, cancel, nil
}

// Get retrieves and verifies a content-addressed object. The body is bounded
// before it is read, so a corrupt or misconfigured object cannot exhaust RAM.
func (b *Backend) Get(ctx context.Context, key string) (Object, error) {
	objectKey, err := b.objectKey(key)
	if err != nil {
		return Object{}, err
	}
	opCtx, cancel, err := b.operation(ctx)
	if err != nil {
		return Object{}, err
	}
	defer cancel()
	out, err := b.client.GetObject(opCtx, &awss3.GetObjectInput{Bucket: aws.String(b.config.Bucket), Key: aws.String(objectKey), ChecksumMode: types.ChecksumModeEnabled})
	if err != nil {
		if isNotFound(err) {
			return Object{}, ErrNotFound
		}
		return Object{}, fmt.Errorf("get cache object: %w", err)
	}
	if out == nil || out.Body == nil {
		return Object{}, fmt.Errorf("get cache object: empty body")
	}
	defer out.Body.Close()
	content, err := io.ReadAll(io.LimitReader(out.Body, b.config.MaxObjectSize+1))
	if err != nil {
		return Object{}, fmt.Errorf("read cache object: %w", err)
	}
	if int64(len(content)) > b.config.MaxObjectSize {
		return Object{}, ErrObjectTooLarge
	}
	digest := sha256.Sum256(content)
	digestHex := hex.EncodeToString(digest[:])
	if digestHex != key {
		return Object{}, fmt.Errorf("%w: key %q does not match object digest", ErrChecksumMismatch, key)
	}
	if out.ContentLength != nil && *out.ContentLength != int64(len(content)) {
		return Object{}, fmt.Errorf("%w: content length metadata is %d, body is %d", ErrChecksumMismatch, *out.ContentLength, len(content))
	}
	if declared := out.Metadata["sha256"]; declared != "" && declared != digestHex {
		return Object{}, fmt.Errorf("%w: sha256 user metadata does not match body", ErrChecksumMismatch)
	}
	if declared := out.Metadata["size"]; declared != "" && declared != fmt.Sprintf("%d", len(content)) {
		return Object{}, fmt.Errorf("%w: size user metadata does not match body", ErrChecksumMismatch)
	}
	if out.ChecksumSHA256 != nil && *out.ChecksumSHA256 != base64.StdEncoding.EncodeToString(digest[:]) {
		return Object{}, fmt.Errorf("%w: S3 checksum does not match body", ErrChecksumMismatch)
	}
	return Object{Key: key, Content: content, Size: int64(len(content)), ContentType: aws.ToString(out.ContentType), ChecksumSHA256: digestHex, ETag: aws.ToString(out.ETag)}, nil
}

// Put stores content under its SHA-256 digest. The key must be the lowercase
// hexadecimal SHA-256 digest of content. Existing objects are never replaced.
func (b *Backend) Put(ctx context.Context, key string, content []byte, contentType string) error {
	objectKey, err := b.objectKey(key)
	if err != nil {
		return err
	}
	if int64(len(content)) > b.config.MaxObjectSize {
		return ErrObjectTooLarge
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != key {
		return fmt.Errorf("%w: key %q does not match content digest", ErrChecksumMismatch, key)
	}
	if strings.ContainsAny(contentType, "\r\n") {
		return fmt.Errorf("%w: content type contains a newline", ErrInvalidConfig)
	}
	opCtx, cancel, err := b.operation(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	in := &awss3.PutObjectInput{
		Bucket: aws.String(b.config.Bucket), Key: aws.String(objectKey), Body: bytes.NewReader(content),
		ContentLength: aws.Int64(int64(len(content))), ContentType: optionalString(contentType),
		ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(digest[:])),
		IfNoneMatch:    aws.String("*"), Metadata: map[string]string{"sha256": hex.EncodeToString(digest[:]), "size": fmt.Sprintf("%d", len(content))},
	}
	if b.config.ServerSideEncryption != "" {
		in.ServerSideEncryption = types.ServerSideEncryption(b.config.ServerSideEncryption)
	}
	if b.config.SSEKMSKeyID != "" {
		in.SSEKMSKeyId = aws.String(b.config.SSEKMSKeyID)
	}
	_, err = b.client.PutObject(opCtx, in)
	if err != nil {
		if isPreconditionFailed(err) {
			return ErrImmutable
		}
		return fmt.Errorf("put cache object: %w", err)
	}
	return nil
}

// Delete removes an object. S3's delete operation is intentionally treated as
// idempotent, including deletion of an object that is already absent.
func (b *Backend) Delete(ctx context.Context, key string) error {
	objectKey, err := b.objectKey(key)
	if err != nil {
		return err
	}
	opCtx, cancel, err := b.operation(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	_, err = b.client.DeleteObject(opCtx, &awss3.DeleteObjectInput{Bucket: aws.String(b.config.Bucket), Key: aws.String(objectKey)})
	if err != nil {
		return fmt.Errorf("delete cache object: %w", err)
	}
	return nil
}

func (b *Backend) objectKey(key string) (string, error) {
	if !digestPattern.MatchString(key) {
		return "", fmt.Errorf("%w: key must be a lowercase %d-character SHA-256 digest", ErrInvalidKey, sha256HexLength)
	}
	if b.config.Prefix == "" {
		return key, nil
	}
	return b.config.Prefix + "/" + key, nil
}

func validatePrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if len(prefix) > 900 || strings.HasPrefix(prefix, "/") || strings.HasSuffix(prefix, "/") || strings.Contains(prefix, "\\") || strings.Contains(prefix, "..") || strings.ContainsAny(prefix, "\r\n") {
		return errors.New("prefix must be relative, traversal-free, and not end with a slash")
	}
	for _, part := range strings.Split(prefix, "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\x00") {
			return errors.New("prefix cannot contain empty path components")
		}
	}
	return nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return aws.String(value)
}

func isNotFound(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NotFound")
}

func isPreconditionFailed(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && (apiErr.ErrorCode() == "PreconditionFailed" || apiErr.ErrorCode() == "ConditionalRequestConflict")
}
