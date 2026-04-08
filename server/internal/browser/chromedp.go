package browser

import (
	"context"
	"os"

	"github.com/chromedp/chromedp"
)

// Browser instance
type Browser struct {
	allocCtx context.Context
	allocCancel context.CancelFunc
}

// NewBrowser initializes a shared Chrome allocator instance.
func NewBrowser() *Browser {
	opts := chromedp.DefaultExecAllocatorOptions[:]

	if os.Getenv("CHROMEDP_HEADLESS") != "false" {
		opts = append(opts, chromedp.Flag("headless", "new"))
	}
	if os.Getenv("CHROMEDP_DISABLE_GPU") == "true" {
		opts = append(opts, chromedp.Flag("disable-gpu", true))
	}
	
	// Default flags required for WebRTC/Media without prompts
	opts = append(opts,
		chromedp.Flag("use-fake-ui-for-media-stream", true),
		chromedp.Flag("use-fake-device-for-media-stream", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"), // avoid detection
	)

	if chromePath := os.Getenv("CHROME_PATH"); chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)

	return &Browser{
		allocCtx:    allocCtx,
		allocCancel: cancel,
	}
}

// Close the allocator
func (b *Browser) Close() {
	if b.allocCancel != nil {
		b.allocCancel()
	}
}

// NewTabContext creates a new tab with isolation (incognito like)
func (b *Browser) NewTabContext(parentCtx context.Context) (context.Context, context.CancelFunc) {
	// Create context
	ctx, cancel := chromedp.NewContext(
		b.allocCtx,
		chromedp.WithLogf(func(s string, args ...interface{}) {
			// Uncomment for debug
			// log.Printf("[cdp-debug] "+s, args...)
		}),
	)
	
	return ctx, cancel
}
