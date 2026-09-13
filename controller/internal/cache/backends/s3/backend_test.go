package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type mockS3 struct {
	getInput    *awss3.GetObjectInput
	putInput    *awss3.PutObjectInput
	deleteInput *awss3.DeleteObjectInput
	getOutput   *awss3.GetObjectOutput
	getErr      error
	putErr      error
	deleteErr   error
	seenContext context.Context
}

func (m *mockS3) GetObject(ctx context.Context, input *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	m.seenContext = ctx
	m.getInput = input
	return m.getOutput, m.getErr
}
func (m *mockS3) PutObject(ctx context.Context, input *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	m.seenContext = ctx
	m.putInput = input
	return &awss3.PutObjectOutput{}, m.putErr
}
func (m *mockS3) DeleteObject(ctx context.Context, input *awss3.DeleteObjectInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error) {
	m.seenContext = ctx
	m.deleteInput = input
	return &awss3.DeleteObjectOutput{}, m.deleteErr
}

type closeBuffer struct{ *bytes.Reader }

func (closeBuffer) Close() error { return nil }

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return strings.ToLower(fmtHex(sum[:]))
}

func fmtHex(v []byte) string {
	const hexChars = "0123456789abcdef"
	var b strings.Builder
	for _, x := range v {
		b.WriteByte(hexChars[x>>4])
		b.WriteByte(hexChars[x&15])
	}
	return b.String()
}

func testBackend(t *testing.T, client *mockS3, config Config) *Backend {
	t.Helper()
	b, err := New(client, config)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestConfigValidation(t *testing.T) {
	client := &mockS3{}
	cases := []Config{
		{},
		{Bucket: "Bucket-With-Uppercase"},
		{Bucket: "cache", Prefix: "../unsafe"},
		{Bucket: "cache", Prefix: "/absolute"},
		{Bucket: "cache", MaxObjectSize: -1},
		{Bucket: "cache", SSEKMSKeyID: "key"},
		{Bucket: "cache", ServerSideEncryption: "unsupported"},
	}
	for _, config := range cases {
		if _, err := New(client, config); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("New(%+v) error = %v, want ErrInvalidConfig", config, err)
		}
	}
	if _, err := New(client, Config{Bucket: "ab"}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("short bucket error = %v", err)
	}
}

func TestPutUsesContentAddressedKeyMetadataAndEncryption(t *testing.T) {
	client := &mockS3{}
	b := testBackend(t, client, Config{Bucket: "cache-bucket", Prefix: "immutable/v1", ServerSideEncryption: "aws:kms", SSEKMSKeyID: "arn:aws:kms:us-east-1:123456789012:key/example"})
	content := []byte("cache payload")
	key := digest(content)
	if err := b.Put(context.Background(), key, content, "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(client.putInput.Bucket); got != "cache-bucket" {
		t.Errorf("bucket = %q", got)
	}
	if got := aws.ToString(client.putInput.Key); got != "immutable/v1/"+key {
		t.Errorf("key = %q", got)
	}
	if got := aws.ToString(client.putInput.IfNoneMatch); got != "*" {
		t.Errorf("IfNoneMatch = %q", got)
	}
	if got := aws.ToString(client.putInput.ContentType); got != "application/octet-stream" {
		t.Errorf("content type = %q", got)
	}
	if got := client.putInput.Metadata["sha256"]; got != key {
		t.Errorf("sha256 metadata = %q", got)
	}
	if got := client.putInput.Metadata["size"]; got != "13" {
		t.Errorf("size metadata = %q", got)
	}
	if got := string(readBody(t, client.putInput.Body)); got != string(content) {
		t.Errorf("body = %q", got)
	}
	if got := aws.ToString(client.putInput.ChecksumSHA256); got != base64.StdEncoding.EncodeToString(sha256Bytes(content)) {
		t.Errorf("checksum = %q", got)
	}
	if client.putInput.ServerSideEncryption != "aws:kms" || aws.ToString(client.putInput.SSEKMSKeyId) == "" {
		t.Fatalf("encryption headers missing: %+v", client.putInput)
	}
}

func TestPutRejectsWrongDigestAndImmutableConflict(t *testing.T) {
	client := &mockS3{}
	b := testBackend(t, client, Config{Bucket: "cache-bucket"})
	if err := b.Put(context.Background(), strings.Repeat("0", 64), []byte("payload"), ""); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("wrong digest error = %v", err)
	}
	client.putErr = &smithy.GenericAPIError{Code: "PreconditionFailed", Message: "already exists"}
	content := []byte("payload")
	if err := b.Put(context.Background(), digest(content), content, ""); !errors.Is(err, ErrImmutable) {
		t.Fatalf("immutable error = %v", err)
	}
}

func TestGetVerifiesBodyAndMetadata(t *testing.T) {
	client := &mockS3{}
	content := []byte("cache payload")
	sum := sha256.Sum256(content)
	client.getOutput = &awss3.GetObjectOutput{Body: closeBuffer{bytes.NewReader(content)}, ContentLength: aws.Int64(int64(len(content))), ContentType: aws.String("text/plain"), ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(sum[:])), ETag: aws.String("etag"), Metadata: map[string]string{"sha256": digest(content), "size": "13"}}
	b := testBackend(t, client, Config{Bucket: "cache-bucket", Prefix: "pfx"})
	key := digest(content)
	got, err := b.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != key || got.Size != int64(len(content)) || string(got.Content) != string(content) || got.ContentType != "text/plain" || got.ChecksumSHA256 != key || got.ETag != "etag" {
		t.Fatalf("unexpected object: %+v", got)
	}
	if aws.ToString(client.getInput.Key) != "pfx/"+key {
		t.Errorf("get key = %q", aws.ToString(client.getInput.Key))
	}
	if client.getInput.ChecksumMode != types.ChecksumModeEnabled {
		t.Errorf("checksum mode = %q", client.getInput.ChecksumMode)
	}
}

func TestGetRejectsChecksumMismatchAndSizeLimit(t *testing.T) {
	content := []byte("not the key")
	client := &mockS3{getOutput: &awss3.GetObjectOutput{Body: closeBuffer{bytes.NewReader(content)}}}
	b := testBackend(t, client, Config{Bucket: "cache-bucket", MaxObjectSize: int64(len(content))})
	if _, err := b.Get(context.Background(), strings.Repeat("0", 64)); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("checksum error = %v", err)
	}
	client.getOutput.Body = closeBuffer{bytes.NewReader([]byte("too large"))}
	b = testBackend(t, client, Config{Bucket: "cache-bucket", MaxObjectSize: 3})
	if _, err := b.Get(context.Background(), digest([]byte("too large"))); !errors.Is(err, ErrObjectTooLarge) {
		t.Fatalf("size error = %v", err)
	}
}

func TestGetNotFoundAndDeleteAreMapped(t *testing.T) {
	client := &mockS3{getErr: &smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}}
	b := testBackend(t, client, Config{Bucket: "cache-bucket"})
	if _, err := b.Get(context.Background(), strings.Repeat("a", 64)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("not found error = %v", err)
	}
	if err := b.Delete(context.Background(), strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if aws.ToString(client.deleteInput.Key) != strings.Repeat("a", 64) {
		t.Errorf("delete key = %q", aws.ToString(client.deleteInput.Key))
	}
}

func TestOperationsHonorCancellationAndTimeout(t *testing.T) {
	client := &mockS3{}
	b := testBackend(t, client, Config{Bucket: "cache-bucket", OperationTimeout: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.Delete(ctx, strings.Repeat("a", 64)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if err := b.Delete(context.Background(), strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if client.seenContext == nil {
		t.Fatal("mock did not receive context")
	}
	select {
	case <-client.seenContext.Done():
	default:
		t.Fatal("operation context has no deadline")
	}
}

func readBody(t *testing.T, body io.Reader) []byte {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func sha256Bytes(content []byte) []byte {
	sum := sha256.Sum256(content)
	return sum[:]
}
