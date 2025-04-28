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
	numWorkers   = 8               // Number of parallel downloads
	maxRetries   = 3               // Max retry attempts per part
	retryBackoff = 2 * time.Second // Wait time before retry
)

var (
	bar *progressbar.ProgressBar
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: downloader download <url>")
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args

	switch cmd {
	case "download":
		url := args[2]

		splitText := strings.Split(url, "/")
		textLen := len(splitText)
		fileName := splitText[textLen-1]

		fs := flag.NewFlagSet("download", flag.ExitOnError)
		output := fs.String("output", fileName, "specify output location")
		fs.Parse(args[3:])

		err := downloadFile(url, *output, numWorkers)
		if err != nil {
			fmt.Println("\nDownload failed:", err)
		} else {
			fmt.Println("\nDownload completed successfully!")
		}
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		fmt.Println("Available commands: download")
	}
}

func downloadFile(url, output string, workers int) error {
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

	// Partial content must respond with 206
	if resp.StatusCode == http.StatusPartialContent {
		return true, nil
	}
	return false, nil
}
