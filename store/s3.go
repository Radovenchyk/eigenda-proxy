package store

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"path"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/minio/minio-go/v7"

	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	S3CredentialStatic  S3CredentialType = "static"
	S3CredentialIAM     S3CredentialType = "iam"
	S3CredentialUnknown S3CredentialType = "unknown"
)

func StringToS3CredentialType(s string) S3CredentialType {
	switch s {
	case "static":
		return S3CredentialStatic
	case "iam":
		return S3CredentialIAM
	default:
		return S3CredentialUnknown
	}
}

var _ PrecomputedKeyStore = (*S3Store)(nil)

type S3CredentialType string
type S3Config struct {
	S3CredentialType S3CredentialType
	Bucket           string
	Path             string
	Endpoint         string
	AccessKeyID      string
	AccessKeySecret  string
	Profiling        bool
	Backup           bool
	Timeout          time.Duration
}

type S3Store struct {
	cfg    S3Config
	client *minio.Client
	stats  *Stats
}

func NewS3(cfg S3Config) (*S3Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  creds(cfg),
		Secure: false,
	})
	if err != nil {
		return nil, err
	}

	return &S3Store{
		cfg:    cfg,
		client: client,
		stats: &Stats{
			Entries: 0,
			Reads:   0,
		},
	}, nil
}

func (s *S3Store) Get(ctx context.Context, key []byte) ([]byte, error) {
	result, err := s.client.GetObject(ctx, s.cfg.Bucket, path.Join(s.cfg.Path, hex.EncodeToString(key)), minio.GetObjectOptions{})
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" {
			return nil, errors.New("value not found in s3 bucket")
		}
		return nil, err
	}
	defer result.Close()
	data, err := io.ReadAll(result)
	if err != nil {
		return nil, err
	}

	if s.cfg.Profiling {
		s.stats.Reads++
	}

	return data, nil
}

func (s *S3Store) Put(ctx context.Context, key []byte, value []byte) error {
	_, err := s.client.PutObject(ctx, s.cfg.Bucket, path.Join(s.cfg.Path, hex.EncodeToString(key)), bytes.NewReader(value), int64(len(value)), minio.PutObjectOptions{})
	if err != nil {
		return err
	}

	if s.cfg.Profiling {
		s.stats.Entries++
	}

	return nil
}

func (s *S3Store) Verify(key []byte, value []byte) error {
	h := crypto.Keccak256Hash(value)
	if !bytes.Equal(h[:], key) {
		return errors.New("key does not match value")
	}

	return nil
}

func (s *S3Store) Stats() *Stats {
	return s.stats
}

func (s *S3Store) BackendType() BackendType {
	return S3
}

func creds(cfg S3Config) *credentials.Credentials {
	if cfg.S3CredentialType == S3CredentialIAM {
		return credentials.NewIAM("")
	}
	return credentials.NewStaticV4(cfg.AccessKeyID, cfg.AccessKeySecret, "")
}
