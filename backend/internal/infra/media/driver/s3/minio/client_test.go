package minio

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// 官方固定向量：https://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-header-based-auth.html
func TestAWSHeaderSigningVector(t *testing.T) {
	c, err := New("https://examplebucket.s3.amazonaws.com", &Options{AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) }
	req, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	if err := c.signRequest(req, "/test.txt", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(req.Header.Get("Authorization"), "Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41") {
		t.Fatal("signature does not match AWS published vector")
	}
}

// 官方固定向量：https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html
func TestAWSPresigningVector(t *testing.T) {
	c, _ := New("https://examplebucket.s3.amazonaws.com", &Options{AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", Region: "us-east-1"})
	c.now = func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) }
	u, err := c.PresignedGetObject(context.Background(), "", "test.txt", 24*time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("X-Amz-Signature") != "aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404" {
		t.Fatal("presigned signature does not match AWS published vector")
	}
}

func TestLiveMinIOReadWriteAndPresignedURLs(t *testing.T) {
	endpoint := os.Getenv("POWERX_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set POWERX_TEST_S3_* for real MinIO acceptance")
	}
	c, err := New(endpoint, &Options{AccessKey: os.Getenv("POWERX_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("POWERX_TEST_S3_SECRET_KEY"), Region: os.Getenv("POWERX_TEST_S3_REGION"), ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bucket := os.Getenv("POWERX_TEST_S3_BUCKET")
	key := "agent-runtime/probes/go-" + time.Now().UTC().Format("20060102T150405.000000000") + "-测试.json"
	payload := []byte(`{"probe":"signed-read-write"}`)
	defer func() {
		if err := c.RemoveObject(context.Background(), bucket, key, RemoveObjectOptions{}); err != nil {
			t.Error("probe cleanup failed")
		}
	}()
	if _, err = c.PutObject(ctx, bucket, key, bytes.NewReader(payload), int64(len(payload)), PutObjectOptions{ContentType: "application/json"}); err != nil {
		t.Fatal(err)
	}
	o, err := c.GetObject(ctx, bucket, key, GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(o)
	_ = o.Close()
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("readback differs")
	}
	u, err := c.PresignedGetObject(ctx, bucket, key, time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("presigned read failed")
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != 200 || !bytes.Equal(body, payload) {
		t.Fatalf("presigned read status=%d", response.StatusCode)
	}
	u, err = c.PresignedPutObject(ctx, bucket, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(payload))
	response, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("presigned write failed")
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("presigned write status=%d", response.StatusCode)
	}
}
