package downloader

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sojebsikder/go-idm/internal/progress"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxRetries     = 3
	retryBackoff   = 2 * time.Second
	maxConcurrency = 8 // limit concurrent connections
)

type DownloadFileOption struct {
	Ctx             context.Context
	Bar             *progress.Bar
	Url             string
	Output          string
	WorkersOverride int
}

func DownloadFile(opt DownloadFileOption) error {
	transport := &http.Transport{
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     100,
	}
	client := &http.Client{Transport: transport}

	// Request first byte to check partial support
	req, err := http.NewRequestWithContext(opt.Ctx, "GET", opt.Url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// If server doesn't support range requests
	if resp.StatusCode != http.StatusPartialContent {
		fmt.Println("Server does not support partial downloads, using single-thread mode...")
		return SingleDownload(opt.Bar, opt.Url, opt.Output)
	}

	contentRange := resp.Header.Get("Content-Range")
	parts := strings.Split(contentRange, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid Content-Range format")
	}
	size, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid content length: %v", err)
	}

	fmt.Printf("File size: %d bytes\n", size)

	workers := CalculateWorkers(size)
	if opt.WorkersOverride > 0 {
		workers = opt.WorkersOverride
	}
	fmt.Printf("Using %d workers (max %d concurrent)...\n", workers, maxConcurrency)

	file, err := os.Create(opt.Output)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := file.Truncate(int64(size)); err != nil {
		return err
	}

	opt.Bar.Start(size)
	defer opt.Bar.Finish()

	partSize := size / workers
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)
	errChan := make(chan error, workers)

	for i := 0; i < workers; i++ {
		start := i * partSize
		end := start + partSize - 1
		if i == workers-1 {
			end = size - 1
		}

		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			for retries := 0; retries <= maxRetries; retries++ {
				select {
				case <-opt.Ctx.Done():
					fmt.Printf("\n Canceled part %d-%d\n", start, end)
					return
				default:
				}

				err := DownloadPart(DownloadPartOption{
					Ctx:    opt.Ctx,
					Client: client,
					Bar:    opt.Bar,
					Url:    opt.Url,
					Output: opt.Output,
					Start:  start,
					End:    end,
				})

				if err == nil {
					return
				}

				fmt.Printf("\nRetrying part %d-%d (attempt %d): %v\n", start, end, retries+1, err)
				if retries == maxRetries {
					fmt.Printf("Failed part %d-%d after %d attempts\n", start, end, maxRetries)
					errChan <- err
					return
				}

				backoff := retryBackoff * time.Duration(1<<retries)
				jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
				time.Sleep(backoff + jitter)
			}
		}(start, end)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for e := range errChan {
		if e != nil {
			return e
		}
	}

	// Check if canceled
	select {
	case <-opt.Ctx.Done():
		return opt.Ctx.Err()
	default:
	}

	return nil
}

// Single-threaded fallback
func SingleDownload(bar *progress.Bar, url, output string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status code %d", resp.StatusCode)
	}

	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()

	bar.Start64(resp.ContentLength)
	defer bar.Finish()

	_, err = io.Copy(io.MultiWriter(file, bar.GetProgressbar()), resp.Body)
	return err
}
