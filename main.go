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

var bar = progress.New()

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
	defaultFileName := downloader.GetFileNameFromURL(url)

	fs := flag.NewFlagSet("download", flag.ExitOnError)
	output := fs.String("output", defaultFileName, "specify output location")
	workersFlag := fs.Int("worker", 0, "override number of workers")
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
	fmt.Printf("Downloaded in: %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Average speed: %s/s\n", downloader.FormatSpeed(speed))
}
