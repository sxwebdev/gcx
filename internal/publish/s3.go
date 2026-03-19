package publish

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/sxwebdev/gcx/internal/config"
	"github.com/sxwebdev/gcx/internal/tmpl"
)

// S3Publisher uploads artifacts to S3-compatible storage.
type S3Publisher struct {
	name      string
	bucket    string
	region    string
	endpoint  string
	directory string
}

// NewS3Publisher creates an S3Publisher from config.
func NewS3Publisher(cfg config.BlobConfig) (*S3Publisher, error) {
	return &S3Publisher{
		name:      cfg.Name,
		bucket:    cfg.Bucket,
		region:    cfg.Region,
		endpoint:  cfg.Endpoint,
		directory: cfg.Directory,
	}, nil
}

func (p *S3Publisher) Name() string { return p.name }

func (p *S3Publisher) Publish(ctx context.Context, artifactsDir string, version string) error {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKey == "" || secretKey == "" {
		return fmt.Errorf("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set")
	}

	remoteDir, err := tmpl.Process("directory", p.directory, map[string]string{"Version": version})
	if err != nil {
		return fmt.Errorf("process directory template: %w", err)
	}

	urlData, err := url.Parse(p.endpoint)
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}

	client, err := minio.New(urlData.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: strings.HasPrefix(p.endpoint, "https"),
		Region: p.region,
	})
	if err != nil {
		return fmt.Errorf("create S3 client: %w", err)
	}

	exists, err := client.BucketExists(ctx, p.bucket)
	if err != nil {
		return fmt.Errorf("bucket check: %w", err)
	}
	if !exists {
		log.Printf("Bucket %s does not exist, creating...", p.bucket)
		if err := client.MakeBucket(ctx, p.bucket, minio.MakeBucketOptions{Region: p.region}); err != nil {
			return fmt.Errorf("create bucket: %w", err)
		}
	}

	files, err := os.ReadDir(artifactsDir)
	if err != nil {
		return fmt.Errorf("read directory %s: %w", artifactsDir, err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		localFilePath := filepath.Join(artifactsDir, file.Name())
		// Use path.Join (not filepath.Join) for URL-style S3 paths
		remotePath := path.Join(remoteDir, file.Name())

		log.Printf("Uploading %s to s3://%s/%s", localFilePath, p.bucket, remotePath)

		f, err := os.Open(localFilePath)
		if err != nil {
			return fmt.Errorf("open file %s: %w", localFilePath, err)
		}

		stat, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("stat file %s: %w", localFilePath, err)
		}

		_, err = client.PutObject(ctx, p.bucket, remotePath, f, stat.Size(), minio.PutObjectOptions{})
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("upload file %s: %w", localFilePath, err)
		}
	}
	return nil
}
