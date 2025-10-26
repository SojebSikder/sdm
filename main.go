package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sojebsikder/go-idm/internal/downloader"
	"sojebsikder/go-idm/internal/progress"
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
	fs.Parse(args[1:])

	fi, err := os.Stat(*output)
	if err == nil && fi.IsDir() {
		*output = filepath.Join(*output, defaultFileName)
	}

	startTime := time.Now()

	err = downloader.DownloadFile(bar, url, *output, *workersFlag)
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
		fmt.Printf("Average speed: %s/s\n", downloader.FormatSpeed(speed))
	}
}
