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

// PresignGet returns a temporary download URL, for objects in a private bucket.
func (r *R2) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := r.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// Size returns an object's size in bytes, and false if it doesn't exist.
func (r *R2) Size(ctx context.Context, key string) (int64, bool, error) {
	out, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) && re.HTTPStatusCode() == 404 {
			return 0, false, nil
		}
		return 0, false, err
	}
	return aws.ToInt64(out.ContentLength), true, nil
}

// Drawings live in the media bucket under drawings/, one folder per drawing. The folder name
// carries a random token (see store.Drawing.Folder) because the media bucket is public.
func DrawingSceneKey(folder string) string   { return folder + "scene.excalidraw" }
func DrawingPreviewKey(folder string) string { return folder + "preview.png" }

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
	case ".m4a":
		return "audio/mp4" // the OS mime table varies (audio/x-m4a, or missing)
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

// CORS returns the bucket's CORS rules as "origins → methods" lines (none if unset).
func (r *R2) CORS(ctx context.Context) ([]string, error) {
	out, err := r.client.GetBucketCors(ctx, &s3.GetBucketCorsInput{Bucket: aws.String(r.bucket)})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchCORSConfiguration") {
			return nil, nil
		}
		return nil, err
	}
	var lines []string
	for _, rule := range out.CORSRules {
		lines = append(lines, strings.Join(rule.AllowedOrigins, ", ")+" → "+strings.Join(rule.AllowedMethods, ", "))
	}
	return lines, nil
}

// SetCORS lets browsers on these origins upload (presigned PUT) and read (GET, HEAD with Range
// for audio and video seeking). It replaces any existing rules.
func (r *R2) SetCORS(ctx context.Context, origins []string) error {
	_, err := r.client.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(r.bucket),
		CORSConfiguration: &types.CORSConfiguration{CORSRules: []types.CORSRule{{
			AllowedOrigins: origins,
			AllowedMethods: []string{"GET", "HEAD", "PUT"},
			AllowedHeaders: []string{"Content-Type", "Range"},
			ExposeHeaders:  []string{"ETag", "Content-Length", "Content-Range"},
			MaxAgeSeconds:  aws.Int32(3600),
		}}},
	})
	return err
}

// Get reads a whole (small) object into memory, refusing anything over max bytes.
func (r *R2) Get(ctx context.Context, key string, max int64) ([]byte, error) {
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	b, err := io.ReadAll(io.LimitReader(out.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%s is over %d bytes", key, max)
	}
	return b, nil
}

// Copy duplicates an object inside the bucket (no download), e.g. a code commit snapshot.
func (r *R2) Copy(ctx context.Context, from, to string) error {
	_, err := r.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(r.bucket),
		CopySource: aws.String(r.bucket + "/" + from),
		Key:        aws.String(to),
	})
	return err
}

// Delete removes one object.
func (r *R2) Delete(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	return err
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
