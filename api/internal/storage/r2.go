// Package storage talks to Cloudflare R2 through its S3-compatible API.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type R2 struct {
	bucket  string
	client  *s3.Client
	presign *s3.PresignClient
}

// NewR2 connects to the bucket. endpoint may be "" to use Cloudflare's default for the account.
func NewR2(accountID, endpoint, accessKey, secretKey, bucket string) *R2 {
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	}
	cfg := aws.Config{
		Region:      "auto",
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	return &R2{bucket: bucket, client: client, presign: s3.NewPresignClient(client)}
}

// Object keys used across the system.
func SourcePrefix(filmID int64) string { return fmt.Sprintf("sources/%d/", filmID) }
func SourceKey(filmID int64) string    { return SourcePrefix(filmID) + "source" }
func HLSPrefix(filmID int64) string    { return fmt.Sprintf("hls/%d/", filmID) }
func MasterKey(filmID int64) string    { return HLSPrefix(filmID) + "master.m3u8" }
func SubtitleKey(filmID int64, lang string) string {
	return fmt.Sprintf("%ssubs/%s.vtt", HLSPrefix(filmID), lang)
}

// PresignPut returns a URL the browser can PUT a file to directly (max 5 GB per PUT).
func (r *R2) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, error) {
	req, err := r.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// Exists reports whether an object exists.
func (r *R2) Exists(ctx context.Context, key string) (bool, error) {
	_, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) && re.HTTPStatusCode() == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Download streams an object to a local file.
func (r *R2) Download(ctx context.Context, key, dst string) error {
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, out.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func contentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	case ".vtt":
		return "text/vtt"
	}
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// UploadDir uploads every file under dir to prefix, keeping relative paths.
func (r *R2) UploadDir(ctx context.Context, dir, prefix string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:       aws.String(r.bucket),
			Key:          aws.String(prefix + filepath.ToSlash(rel)),
			Body:         f,
			ContentType:  aws.String(contentType(path)),
			CacheControl: aws.String("public, max-age=31536000, immutable"),
		})
		return err
	})
}

// DeletePrefix removes every object under prefix.
func (r *R2) DeletePrefix(ctx context.Context, prefix string) error {
	p := s3.NewListObjectsV2Paginator(r.client, &s3.ListObjectsV2Input{Bucket: aws.String(r.bucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		if len(page.Contents) == 0 {
			continue
		}
		ids := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, o := range page.Contents {
			ids = append(ids, types.ObjectIdentifier{Key: o.Key})
		}
		if _, err := r.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(r.bucket),
			Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		}); err != nil {
			return err
		}
	}
	return nil
}
