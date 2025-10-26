package downloader

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sojebsikder/go-idm/internal/progress"
)

func DownloadPart(client *http.Client, bar *progress.Bar, url string, output string, start int, end int) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("unexpected status %d for range %d-%d", resp.StatusCode, start, end)
	}

	file, err := os.OpenFile(output, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Seek(int64(start), io.SeekStart); err != nil {
		return err
	}

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				return err
			}
			bar.Add(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}
