package progress

import (
	"time"

	"github.com/schollz/progressbar/v3"
)

type Bar struct {
	bar *progressbar.ProgressBar
}

func New() *Bar {
	return &Bar{}
}

func (b *Bar) GetProgressbar() *progressbar.ProgressBar {
	return b.bar
}

func (b *Bar) Start(size int) {
	b.bar = progressbar.NewOptions(size,
		progressbar.OptionSetDescription("Downloading"),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
	)
}

func (b *Bar) Start64(size int64) {
	b.bar = progressbar.NewOptions64(size,
		progressbar.OptionSetDescription("Downloading"),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
	)
}

func (b *Bar) Add(n int) {
	if b.bar != nil {
		b.bar.Add(n)
	}
}

func (b *Bar) Finish() {
	if b.bar != nil {
		b.bar.Close()
	}
}
