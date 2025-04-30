package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

const (
	maxRetries   = 3
	retryBackoff = 2 * time.Second
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

// download command
func downloadCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: sdm download <url> [--output file] [--worker n]")
		os.Exit(1)
	}

	url := args[0]
	defaultFileName := getFileNameFromURL(url)

	fs := flag.NewFlagSet("download", flag.ExitOnError)
	output := fs.String("output", defaultFileName, "specify output location")
	workersFlag := fs.Int("worker", 0, "override number of workers")
	fs.Parse(args[1:])

	// If output is a directory, append filename
	fi, err := os.Stat(*output)
	if err == nil && fi.IsDir() {
		*output = filepath.Join(*output, defaultFileName)
	}

	startTime := time.Now()

	err = downloadFile(url, *output, *workersFlag)
	if err != nil {
		fmt.Println("\nDownload failed:", err)
		os.Exit(1)
	} else {
		elapsed := time.Since(startTime)

		info, err := os.Stat(*output)
		if err != nil {
			fmt.Println("Error getting downloaded file size:", err)
			return
		}
		size := info.Size()
		speed := float64(size) / elapsed.Seconds()

		fmt.Println("\nDownload completed successfully!")
		fmt.Printf("Downloaded in: %s\n", elapsed.Round(time.Millisecond))
		fmt.Printf("Average speed: %s/s\n", formatSpeed(speed))
	}
}

// download file
func downloadFile(url, output string, workersOverride int) error {
	client := &http.Client{}

	// Perform a GET request for bytes 0-0 to detect size and partial support
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
		return singleDownload(url, output)
	}

	contentRange := resp.Header.Get("Content-Range")
	if contentRange == "" {
		return fmt.Errorf("missing Content-Range header")
	}
	parts := strings.Split(contentRange, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid Content-Range format")
	}
	size, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid content length: %v", err)
	}

	fmt.Printf("File size: %d bytes\n", size)

	workers := calculateWorkers(size)
	if workersOverride > 0 {
		workers = workersOverride
	}
	fmt.Printf("Using %d workers...\n", workers)

	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()

	err = file.Truncate(int64(size))
	if err != nil {
		return err
	}

	bar = progressbar.NewOptions(size,
		progressbar.OptionSetDescription("Downloading"),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
	)
	defer bar.Close()

	partSize := size / workers
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		start := i * partSize
		end := start + partSize - 1
		if i == workers-1 {
			end = size - 1
		}

		go func(start, end int) {
			defer wg.Done()
			retries := 0
			for {
				err := downloadPart(client, url, output, start, end)
				if err == nil {
					break
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

// download part of the file
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

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			bar.Add(n)
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

// single download if server does not support partial content
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

// calculate workers based on file size
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

// format speed as MB/s, KB/s, B/s
func formatSpeed(bps float64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bps > GB:
		return fmt.Sprintf("%.2f GB", bps/GB)
	case bps > MB:
		return fmt.Sprintf("%.2f MB", bps/MB)
	case bps > KB:
		return fmt.Sprintf("%.2f KB", bps/KB)
	default:
		return fmt.Sprintf("%.2f B", bps)
	}
}

func getFileNameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "downloaded_file"
	}

	// Try to extract from content-disposition-like param
	if name := u.Query().Get("filename"); name != "" {
		return name
	}

	// Fallback to path basename without query
	base := filepath.Base(u.Path)
	return base
}
