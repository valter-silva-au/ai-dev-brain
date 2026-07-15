package cloudsync

import (
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ObjectStore is the interface the orchestrator (Push / Pull / Status /
// Destroy) depends on. Real deployments plug in *S3Store; unit tests plug
// in an in-memory fake. This is the seam that keeps the package
// unit-testable offline (no AWS account, no network, no credentials).
type ObjectStore interface {
	Put(ctx context.Context, key string, body io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	List(ctx context.Context, prefix string) ([]string, error)
	Delete(ctx context.Context, keys []string) error
}

// NewS3Store builds an S3-backed ObjectStore for the given bucket in
// region. Credentials come from the local AWS profile chain (env / shared
// config / IMDS). No credentials are ever embedded, logged, or persisted
// by this package.
//
// SSE-KMS: this client deliberately does NOT set ServerSideEncryption /
// SSEKMSKeyId on PutObject. The bucket's default encryption (set by CDK)
// applies server-side, so uploads are encrypted without the client
// holding the key id — keeping the client policy minimal.
func NewS3Store(ctx context.Context, bucket, region string) (*S3Store, error) {
	if bucket == "" {
		return nil, errors.New("cloudsync: bucket must not be empty")
	}
	if region == "" {
		return nil, errors.New("cloudsync: region must not be empty")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg)
	return &S3Store{
		bucket:   bucket,
		client:   client,
		uploader: manager.NewUploader(client),
	}, nil
}

// S3Store is the AWS SDK Go v2 implementation of ObjectStore. Kept small
// on purpose — the orchestrator does the interesting work; this file
// just marshals to and from S3.
type S3Store struct {
	bucket   string
	client   *s3.Client
	uploader *manager.Uploader
}

func (s *S3Store) Put(ctx context.Context, key string, body io.Reader) error {
	_, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	return err
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

// List paginates through ListObjectsV2 and returns every key under prefix.
// The paginator handles IsTruncated + ContinuationToken internally, per
// aws-sdk-go-v2 idiom.
func (s *S3Store) List(ctx context.Context, prefix string) ([]string, error) {
	pager := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	var keys []string
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
	}
	return keys, nil
}

// Delete batches keys via DeleteObjects (max 1000 per S3 request; larger
// key sets are chunked).
func (s *S3Store) Delete(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	const maxBatch = 1000
	for start := 0; start < len(keys); start += maxBatch {
		end := start + maxBatch
		if end > len(keys) {
			end = len(keys)
		}
		ids := make([]types.ObjectIdentifier, 0, end-start)
		for _, k := range keys[start:end] {
			ids = append(ids, types.ObjectIdentifier{Key: aws.String(k)})
		}
		_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return err
		}
	}
	return nil
}
