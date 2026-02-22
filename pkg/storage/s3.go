package storage

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config holds S3-specific configuration
type S3Config struct {
	Region     string
	Bucket     string
	AccessKey  string
	SecretKey  string
	Endpoint   string // Optional: for S3-compatible services
	Encryption string // Optional: server-side encryption (AES256, aws:kms)
}

// S3Storage implements Storage interface for AWS S3
type S3Storage struct {
	client *s3.Client
	bucket string
	config S3Config
}

// NewS3Storage creates a new S3 storage instance
func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	ctx := context.Background()

	// Build AWS SDK configuration options
	options := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}

	// Add static credentials if provided
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		creds := credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
		options = append(options, config.WithCredentialsProvider(creds))
	}

	// Load AWS configuration
	awsCfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Build S3 client options
	s3Options := []func(*s3.Options){
		func(o *s3.Options) {
			o.UsePathStyle = true
		},
	}

	// Add custom endpoint if provided (for S3-compatible services like MinIO)
	if cfg.Endpoint != "" {
		s3Options = append(s3Options, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Options...)

	return &S3Storage{
		client: client,
		bucket: cfg.Bucket,
		config: cfg,
	}, nil
}

// Upload uploads a file to S3
func (s *S3Storage) Upload(ctx context.Context, localPath string, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return &StorageError{Message: "failed to open local file", Cause: err}
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return &StorageError{Message: "failed to stat local file", Cause: err}
	}
	if stat.IsDir() {
		return &StorageError{Message: "cannot upload directory directly"}
	}

	// Prepare put object input
	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(remotePath),
		Body:          file,
		ContentLength: aws.Int64(stat.Size()),
	}

	// Add server-side encryption if configured
	if s.config.Encryption != "" {
		input.ServerSideEncryption = types.ServerSideEncryption(s.config.Encryption)
	}

	// Upload the file
	_, err = s.client.PutObject(ctx, input)
	if err != nil {
		return s.convertError(err, "failed to upload file")
	}

	return nil
}

// List lists objects with the given prefix
func (s *S3Storage) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo
	var continuationToken *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		}

		output, err := s.client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, s.convertError(err, "failed to list objects")
		}

		for _, obj := range output.Contents {
			objects = append(objects, ObjectInfo{
				Key:          aws.ToString(obj.Key),
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
				ETag:         aws.ToString(obj.ETag),
			})
		}

		if !aws.ToBool(output.IsTruncated) {
			break
		}
		continuationToken = output.NextContinuationToken
	}

	return objects, nil
}

// Delete removes an object from S3
func (s *S3Storage) Delete(ctx context.Context, remotePath string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(remotePath),
	}

	_, err := s.client.DeleteObject(ctx, input)
	if err != nil {
		return s.convertError(err, "failed to delete object")
	}

	return nil
}

// Validate checks if the S3 credentials are valid by attempting to list buckets
func (s *S3Storage) Validate(ctx context.Context) error {
	// Try to list buckets to verify credentials
	_, err := s.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return s.convertError(err, "failed to validate credentials")
	}

	// Also verify bucket exists and is accessible by attempting to get its location
	_, err = s.client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		return s.convertError(err, "bucket not accessible")
	}

	return nil
}

// convertError converts AWS SDK errors to StorageError
func (s *S3Storage) convertError(err error, message string) error {
	if err == nil {
		return nil
	}

	// Try to extract error details using smithy error interface
	type smithyError interface {
		Error() string
		ErrorCode() string
	}

	if se, ok := err.(smithyError); ok {
		switch se.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return &StorageError{Message: "not found", Cause: err}
		case "InvalidAccessKeyId", "SignatureDoesNotMatch":
			return &StorageError{Message: "invalid credentials", Cause: err}
		case "AccessDenied", "Forbidden":
			return &StorageError{Message: "permission denied", Cause: err}
		case "NoSuchBucket":
			return &StorageError{Message: "bucket not found", Cause: err}
		}
	}

	return &StorageError{Message: message, Cause: err}
}
