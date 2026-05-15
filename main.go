package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sojebsikder/go-idm/internal/downloader"
	"sojebsikder/go-idm/internal/progress"
	"syscall"
	"time"
)

var appName = "sdm"
var appVersion = "0.0.2"

var bar = progress.New()

func main() {

	cmd := os.Args[1]

	switch cmd {
	case "download":
		if len(os.Args) < 3 {
			fmt.Println("Usage: sdm download <url>")
			os.Exit(1)
		}
		downloadCmd(os.Args[2:])
	case "help":
		fmt.Printf("%s is a command-line tool for downloading files from the internet.\n", appName)
		fmt.Printf("It supports downloading multiple files concurrently and provides progress tracking.\n\n")
		fmt.Println("Usage: sdm download <url> [--output file] [--worker n]")
		fmt.Printf("Usage: sdm help\n\n")
		fmt.Printf("(c) sojebsikder <sojebsikder@gmail.com>")
	case "version":
		fmt.Printf("%s version %s\n", appName, appVersion)
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		fmt.Println("Available commands: download")
		os.Exit(1)
	}
}

type headerFlags []string

func (h *headerFlags) String() string {
	return fmt.Sprint(*h)
}

func (h *headerFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

// download command
func downloadCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: sdm download <url> [--output file] [--worker n]")
		os.Exit(1)
	}

	url := args[0]
	defaultFileName := downloader.GetFileNameFromURL(url)

	fs := flag.NewFlagSet("download", flag.ExitOnError)
	output := fs.String("output", defaultFileName, "specify output location")
	workersFlag := fs.Int("worker", 0, "override number of workers")
	// request headers, cookies etc...
	var headers headerFlags
	fs.Var(&headers, "header", "HTTP header (can be used multiple times). Example: --header=\"Authorization: Bearer TOKEN\"")

	userAgent := fs.String("user-agent", "", "HTTP user agent string")
	cookies := fs.String("cookie", "", "HTTP cookie string")
	fs.Parse(args[1:])

	fi, err := os.Stat(*output)
	if err == nil && fi.IsDir() {
		*output = filepath.Join(*output, defaultFileName)
	}

	// Create context that cancels on SIGINT or SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startTime := time.Now()

	// Start download
	err = downloader.DownloadFile(downloader.DownloadFileOption{
		Ctx:             ctx,
		Bar:             bar,
		Url:             url,
		Output:          *output,
		WorkersOverride: *workersFlag,
		Cookies:         *cookies,
		Headers:         headers,
		UserAgent:       *userAgent,
	})

	// Handle result
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Println("\nDownload canceled by user.")
			os.Exit(0)
		}
		fmt.Println("\nDownload failed:", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime)
	info, err := os.Stat(*output)
	if err != nil {
		fmt.Println("Error getting downloaded file size:", err)
		return
	}

	size := info.Size()
	speed := float64(size) / elapsed.Seconds()

	fmt.Println("\n✅ Download completed successfully!")
	fmt.Printf("File size: %s\n", downloader.FormatSpeed(float64(size)))
	fmt.Printf("Downloaded in: %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Average speed: %s/s\n", downloader.FormatSpeed(speed))
}
