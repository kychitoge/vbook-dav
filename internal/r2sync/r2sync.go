package r2sync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// JobAction identifies whether an operation is an upload or a deletion.
type JobAction int

const (
	ActionUpload JobAction = iota
	ActionDelete
)

// Job represents an asynchronous replication task for Cloudflare R2.
type Job struct {
	Action    JobAction
	LocalPath string // Absolute or relative local filesystem path
	R2Key     string // POSIX key without leading slash
}

const (
	defaultQueueCapacity = 64
)

// Syncer orchestrates background synchronization from local disk to Cloudflare R2.
type Syncer struct {
	client  *s3.Client
	bucket  string
	baseDir string
	queue   chan Job
	onDone  func(job Job, err error)
}

// New constructs an R2 Syncer instance. Returns nil if mandatory credentials or endpoint are empty.
func New(endpoint, accessKey, secretKey, bucket, baseDir string) *Syncer {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)
	bucket = strings.TrimSpace(bucket)

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil
	}

	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "auto",
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		UsePathStyle: true,
	})

	return &Syncer{
		client:  client,
		bucket:  bucket,
		baseDir: baseDir,
		queue:   make(chan Job, defaultQueueCapacity),
	}
}

// OnDone registers an optional completion callback. Safe to call on nil receiver.
func (s *Syncer) OnDone(fn func(Job, error)) {
	if s == nil {
		return
	}
	s.onDone = fn
}

// isIgnoredSyncFile determines if a file name or R2 key should be excluded from cloud replication.
// Excludes dot-files (.gitkeep, .DS_Store, .gitignore, etc.) and temporary upload files (*.tmp).
func isIgnoredSyncFile(name string, r2Key string) bool {
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".tmp") {
		return true
	}
	if r2Key != "" {
		for _, part := range strings.Split(r2Key, "/") {
			if strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".tmp") {
				return true
			}
		}
	}
	return false
}

// NormalizeKey converts any path format into a clean S3/R2 POSIX key.
// Explicitly replaces backslashes with slashes (regardless of host OS) and strips leading slashes.
func NormalizeKey(key string) string {
	normalized := strings.ReplaceAll(key, "\\", "/")
	return strings.TrimLeft(normalized, "/")
}

// Enqueue adds a job non-blockingly (used by WebDAV HTTP handlers).
// Returns true if enqueued; returns false and triggers onDone if queue is full.
func (s *Syncer) Enqueue(j Job) bool {
	if s == nil {
		return false
	}
	// Defensively normalize R2 key
	j.R2Key = NormalizeKey(j.R2Key)
	if isIgnoredSyncFile(filepath.Base(j.R2Key), j.R2Key) {
		return false
	}

	select {
	case s.queue <- j:
		return true
	default:
		log.Printf("[warn ] Cloud replication queue is full. Dropped replication task: %s", j.R2Key)
		if s.onDone != nil {
			s.onDone(j, errors.New("sync queue full, job dropped"))
		}
		return false
	}
}

// enqueueBlocking adds a job with context pacing (used by startup scan).
func (s *Syncer) enqueueBlocking(ctx context.Context, j Job) error {
	if s == nil {
		return nil
	}
	j.R2Key = NormalizeKey(j.R2Key)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.queue <- j:
		return nil
	}
}

// Start launches the single worker goroutine and background startup scan.
func (s *Syncer) Start(ctx context.Context) {
	if s == nil {
		return
	}
	go s.worker(ctx)
	go s.startupScan(ctx)
}

// worker sequentially processes replication jobs until context cancellation.
func (s *Syncer) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.queue:
			var err error
			switch job.Action {
			case ActionUpload:
				err = s.upload(ctx, job)
			case ActionDelete:
				err = s.delete(ctx, job)
			}
			if s.onDone != nil {
				s.onDone(job, err)
			}
		}
	}
}

// upload streams a local file to Cloudflare R2 via s3.PutObject.
func (s *Syncer) upload(ctx context.Context, job Job) error {
	f, err := os.Open(job.LocalPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[info ] File was removed before Cloudflare R2 upload: %s", job.LocalPath)
			return nil
		}
		return fmt.Errorf("open %s: %w", job.LocalPath, err)
	}
	defer f.Close()

	var size int64
	if fi, statErr := f.Stat(); statErr == nil {
		size = fi.Size()
	} else {
		log.Printf("[warn ] Could not inspect file metadata for %s: %v", job.LocalPath, statErr)
	}

	log.Printf("[info ] Uploading to Cloudflare R2: %s (%d bytes)", job.R2Key, size)

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(job.R2Key),
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("R2 upload %s: %w", job.R2Key, err)
	}
	return nil
}

// delete issues an idempotent s3.DeleteObject request to Cloudflare R2.
func (s *Syncer) delete(ctx context.Context, job Job) error {
	log.Printf("[info ] Deleting from Cloudflare R2: %s", job.R2Key)

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(job.R2Key),
	})
	if err != nil {
		return fmt.Errorf("R2 delete %s: %w", job.R2Key, err)
	}
	return nil
}

// startupScan reconciles local files against existing R2 bucket keys on startup.
func (s *Syncer) startupScan(ctx context.Context) {
	if _, err := os.Stat(s.baseDir); os.IsNotExist(err) {
		return
	}

	r2Keys := make(map[string]bool)
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Printf("[warn ] Cloudflare R2 startup reconciliation scan failed: %v. Check bucket credentials.", err)
			return
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if key != "" {
				if isIgnoredSyncFile(filepath.Base(key), key) {
					// Purge erroneously uploaded hidden files (e.g. .gitkeep) from R2
					log.Printf("[info ] Removing ignored file from Cloudflare R2: %s", key)
					_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
						Bucket: aws.String(s.bucket),
						Key:    aws.String(key),
					})
					continue
				}
				r2Keys[key] = true
			}
		}
	}

	count := 0
	err := filepath.WalkDir(s.baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip temporary staging files and hidden dot-files
		if isIgnoredSyncFile(d.Name(), "") {
			return nil
		}

		rel, err := filepath.Rel(s.baseDir, path)
		if err != nil {
			return nil
		}
		r2Key := NormalizeKey(rel)
		if isIgnoredSyncFile(filepath.Base(r2Key), r2Key) {
			return nil
		}

		if !r2Keys[r2Key] {
			job := Job{
				Action:    ActionUpload,
				LocalPath: path,
				R2Key:     r2Key,
			}
			if err := s.enqueueBlocking(ctx, job); err != nil {
				return err
			}
			count++
		}
		return nil
	})

	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("[warn ] Local directory walk for Cloudflare R2 reconciliation was interrupted: %v", err)
	}

	if count > 0 {
		log.Printf("[info ] Startup reconciliation queued %d file(s) for Cloudflare R2 upload", count)
	}
}
