package downloader

import (
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
	maxConcurrency = 8 // maximum concurrent HTTP requests
)

func DownloadFile(bar *progress.Bar, url string, output string, workersOverride int) error {
	transport := &http.Transport{
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     100,
	}
	client := &http.Client{Transport: transport}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		fmt.Println("Server does not support partial downloads, falling back to single thread...")
		return SingleDownload(bar, url, output)
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
	if workersOverride > 0 {
		workers = workersOverride
	}
	fmt.Printf("Using %d workers (max %d concurrent)...\n", workers, maxConcurrency)

	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := file.Truncate(int64(size)); err != nil {
		return err
	}

	bar.Start(size)
	defer bar.Finish()

	partSize := size / workers
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)

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
				err := DownloadPart(client, bar, url, output, start, end)
				if err == nil {
					break
				}

				fmt.Printf("\nRetrying part %d-%d (attempt %d): %v\n", start, end, retries+1, err)
				if retries == maxRetries {
					fmt.Printf("Failed part %d-%d after %d attempts\n", start, end, maxRetries)
					break
				}

				backoff := retryBackoff * time.Duration(1<<retries)
				jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
				time.Sleep(backoff + jitter)
			}
		}(start, end)
	}

	wg.Wait()
	return nil
}

func SingleDownload(bar *progress.Bar, url, output string) error {
	resp, err := http.Get(url)
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
