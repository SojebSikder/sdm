package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

const (
	maxRetries   = 3               // Max retry attempts per part
	retryBackoff = 2 * time.Second // Wait time before retry
)

var (
	bar *progressbar.ProgressBar
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: sdm download <url>")
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "download":
		downloadCmd(os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		fmt.Println("Available commands: download")
		os.Exit(1)
	}
}

func downloadCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: sdm download <url> [--output file] [--worker n]")
		os.Exit(1)
	}

	url := args[0]

	// Extract filename from URL
	urlParts := strings.Split(url, "/")
	defaultFileName := urlParts[len(urlParts)-1]

	// Flags
	fs := flag.NewFlagSet("download", flag.ExitOnError)
	output := fs.String("output", defaultFileName, "specify output location")
	workersFlag := fs.Int("worker", 0, "override number of workers")
	fs.Parse(args[1:])

	startTime := time.Now() // capture start time

	err := downloadFile(url, *output, *workersFlag)
	if err != nil {
		fmt.Println("\nDownload failed:", err)
		os.Exit(1)
	} else {
		elapsed := time.Since(startTime) // calculate elapsed time
		fmt.Println("\nDownload completed successfully!")
		fmt.Printf("Downloaded in: %s\n", elapsed.Round(time.Millisecond))
	}
}

func downloadFile(url, output string, workersOverride int) error {
	// Get file size and check for partial support
	resp, err := http.Head(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status code %d", resp.StatusCode)
	}

	sizeStr := resp.Header.Get("Content-Length")
	if sizeStr == "" {
		return fmt.Errorf("missing Content-Length header")
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return fmt.Errorf("invalid content length: %v", err)
	}

	acceptRanges := resp.Header.Get("Accept-Ranges")
	if acceptRanges != "bytes" {
		fmt.Println("Server does not support partial downloads, falling back to single thread...")
		return singleDownload(url, output)
	}

	// Verify actual partial download support
	partialSupported, err := verifyPartialSupport(url)
	if err != nil {
		return fmt.Errorf("error verifying partial download support: %v", err)
	}
	if !partialSupported {
		fmt.Println("Server claims partial download support but does not behave correctly. Falling back to single thread...")
		return singleDownload(url, output)
	}

	fmt.Printf("File size: %d bytes\n", size)

	workers := calculateWorkers(size)
	if workersOverride > 0 {
		workers = workersOverride
	}
	fmt.Printf("Using %d workers...\n", workers)

	// Create output file
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()

	// Pre-allocate file size
	err = file.Truncate(int64(size))
	if err != nil {
		return err
	}

	// Setup the progress bar
	bar = progressbar.NewOptions(size,
		progressbar.OptionSetDescription("Downloading"),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
	)
	defer bar.Close()

	client := &http.Client{}

	// Calculate chunks
	partSize := size / workers

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		start := i * partSize
		end := start + partSize - 1
		if i == workers-1 {
			end = size - 1 // last chunk takes the remainder
		}

		go func(start, end int) {
			defer wg.Done()
			retries := 0
			for {
				err := downloadPart(client, url, output, start, end)
				if err == nil {
					break // Success
				}
				retries++
				if retries > maxRetries {
					fmt.Printf("\nFailed to download part %d-%d after %d retries: %v\n", start, end, maxRetries, err)
					break
				}
				fmt.Printf("\nRetrying part %d-%d (attempt %d)...\n", start, end, retries)
				time.Sleep(retryBackoff)
			}
		}(start, end)
	}

	wg.Wait()
	return nil
}

func downloadPart(client *http.Client, url, output string, start, end int) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	rangeHeader := fmt.Sprintf("bytes=%d-%d", start, end)
	req.Header.Set("Range", rangeHeader)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("server does not support partial content: %d", resp.StatusCode)
	}

	file, err := os.OpenFile(output, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Seek(int64(start), io.SeekStart)
	if err != nil {
		return err
	}

	buf := make([]byte, 32*1024) // 32 KB buffer
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			bar.Add(n) // Update the progress bar
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	return nil
}

func singleDownload(url, output string) error {
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

	bar = progressbar.NewOptions64(resp.ContentLength,
		progressbar.OptionSetDescription("Downloading"),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
	)
	defer bar.Close()

	_, err = io.Copy(io.MultiWriter(file, bar), resp.Body)
	return err
}

// verifyPartialSupport sends a small Range request and checks server behavior
func verifyPartialSupport(url string) (bool, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Range", "bytes=0-1") // Ask for first 2 bytes

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPartialContent {
		return true, nil
	}
	return false, nil
}

// calculateWorkers decides number of workers based on file size
func calculateWorkers(size int) int {
	const (
		MB = 1024 * 1024
		GB = 1024 * MB
	)

	switch {
	case size < 5*MB:
		return 1
	case size < 100*MB:
		return 4
	case size < 1*GB:
		return 8
	default:
		return 16
	}
}
