package storage

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// The examples from the AWS documentation ("Signature Calculations for the
// Authorization Header" and "Authenticating Requests: Using Query
// Parameters"), with the documented credentials.
func exampleBucket(t *testing.T) *S3Bucket {
	t.Helper()
	b, err := NewS3(S3Options{Endpoint: "https://s3.amazonaws.com", Region: "us-east-1", Bucket: "examplebucket",
		AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var exampleTime = time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

func TestSigV4Header(t *testing.T) {
	b := exampleBucket(t)
	req, _ := http.NewRequest(http.MethodGet, b.objectURL("test.txt", nil).String(), nil)
	req.Header.Set("Range", "bytes=0-9")
	b.sign(req, emptyHash, exampleTime)
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization =\n%s\nwant\n%s", got, want)
	}
}

func TestSigV4Presigned(t *testing.T) {
	b := exampleBucket(t)
	got := b.presign(http.MethodGet, "test.txt", 24*time.Hour, exampleTime)
	if !strings.HasSuffix(got, "X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404") {
		t.Errorf("presigned URL = %s", got)
	}
	if !strings.HasPrefix(got, "https://examplebucket.s3.amazonaws.com/test.txt?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20130524T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host") {
		t.Errorf("presigned URL = %s", got)
	}
}

func TestObjectURL(t *testing.T) {
	b, _ := NewS3(S3Options{Endpoint: "http://localhost:9000", Region: "us-east-1", Bucket: "files", AccessKey: "a", SecretKey: "b", PathStyle: true})
	if got := b.objectURL("a b/ü+x.txt", nil).String(); got != "http://localhost:9000/files/a%20b/%C3%BC%2Bx.txt" {
		t.Errorf("url = %s", got)
	}
}
