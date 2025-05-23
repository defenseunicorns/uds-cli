package diagnostic

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

type Uploader interface {
	UploadFile(ctx context.Context, filePath string) error
}

type S3Uploader struct {
	BucketName string
	Region     string
}

func NewS3Uploader(bucketName, region string) *S3Uploader {
	return &S3Uploader{
		BucketName: bucketName,
		Region:     region,
	}
}

func (u *S3Uploader) UploadFile(ctx context.Context, filePath string) error {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(u.Region),
		Credentials: credentials.NewEnvCredentials(),
	})
	if err != nil {
		return fmt.Errorf("failed to create AWS session: %w", err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	uploader := s3manager.NewUploader(sess)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(u.BucketName),
		Key:    aws.String(filePath),
		Body:   file,
	})
	//_, err = s3Client.PutObjectWithContext(ctx, &s3.PutObjectInput{
	//	Bucket: aws.String(u.BucketName),
	//	Key:    aws.String(filePath),
	//	Body:   file,
	//})
	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}

	return nil
}
