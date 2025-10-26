package downloader

import (
	"fmt"
	"net/url"
	"path/filepath"
)

func CalculateWorkers(size int) int {
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

func FormatSpeed(bps float64) string {
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

func GetFileNameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "downloaded_file"
	}
	if name := u.Query().Get("filename"); name != "" {
		return name
	}
	return filepath.Base(u.Path)
}
