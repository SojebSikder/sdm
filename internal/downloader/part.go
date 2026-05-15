package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sojebsikder/go-idm/internal/progress"
)

type DownloadPartOption struct {
	Ctx     context.Context
	Client  *http.Client
	Bar     *progress.Bar
	Url     string
	Output  string
	Start   int
	End     int
	Cookies string
	BufSize *int
}

func DownloadPart(opt DownloadPartOption) error {
	select {
	case <-opt.Ctx.Done():
		return opt.Ctx.Err()
	default:
	}

	req, err := http.NewRequestWithContext(opt.Ctx, "GET", opt.Url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", opt.Start, opt.End))

	if opt.Cookies != "" {
		req.Header.Set("Cookie", opt.Cookies)
	}

	resp, err := opt.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("unexpected status %d for range %d-%d", resp.StatusCode, opt.Start, opt.End)
	}

	file, err := os.OpenFile(opt.Output, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Seek(int64(opt.Start), io.SeekStart); err != nil {
		return err
	}

	size := opt.BufSize
	if size == nil {
		size = new(int)
		*size = 128 * 1024
	}

	buf := make([]byte, *size)
	for {
		select {
		case <-opt.Ctx.Done():
			return opt.Ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				return err
			}
			opt.Bar.Add(n)
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
