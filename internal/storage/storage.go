// Package storage is web-be's S3 client (config/drive.ts minioClient):
// path-style requests to DRIVE_ENDPOINT, objects in DRIVE_BUCKET.
package storage

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// Object describes a PutObjectCommand.
type Object struct {
	ContentType  string
	CacheControl string
	PublicRead   bool
}

type Store interface {
	Put(ctx context.Context, key string, data []byte, o Object) error
	Delete(ctx context.Context, key string) error
	Get(ctx context.Context, key string) ([]byte, string, error)
}

type S3 struct {
	Client *s3.Client
	Bucket string
}

func New(endpoint, region, key, secret, bucket string) *S3 {
	client := s3.New(s3.Options{Region: region, Credentials: credentials.NewStaticCredentialsProvider(key, secret, ""),
		BaseEndpoint: aws.String(endpoint), UsePathStyle: true})
	return &S3{Client: client, Bucket: bucket}
}

func (s *S3) Put(ctx context.Context, key string, data []byte, o Object) error {
	input := &s3.PutObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String(key), Body: bytes.NewReader(data)}
	if o.ContentType != "" {
		input.ContentType = aws.String(o.ContentType)
	}
	if o.CacheControl != "" {
		input.CacheControl = aws.String(o.CacheControl)
	}
	if o.PublicRead {
		input.ACL = types.ObjectCannedACLPublicRead
	}
	_, err := s.Client.PutObject(ctx, input)
	return err
}

func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String(key)})
	return err
}

// ErrNotFound is a missing object.
var ErrNotFound = errors.New("object not found")

func (s *S3) Get(ctx context.Context, key string) ([]byte, string, error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String(key)})
	var api smithy.APIError
	if errors.As(err, &api) && (api.ErrorCode() == "NoSuchKey" || api.ErrorCode() == "NotFound") {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	return data, aws.ToString(out.ContentType), err
}
