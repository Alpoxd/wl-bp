package browser

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// TelemostSession manages a specific call session in Telemost.
type TelemostSession struct {
	browser *Browser
	url     string
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewTelemostSession(b *Browser) *TelemostSession {
	return &TelemostSession{
		browser: b,
	}
}

// SetCookies applies a raw cookie string (e.g. "Session_id=xxx; yandexuid=yyy") to the domain
func setCookiesAction(cookieStr string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		parts := strings.Split(cookieStr, ";")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			keyValue := strings.SplitN(part, "=", 2)
			if len(keyValue) != 2 {
				continue
			}
			
			// We set cookie for yandex.ru and telemost.yandex.ru
			name := strings.TrimSpace(keyValue[0])
			value := strings.TrimSpace(keyValue[1])
			
			err := network.SetCookie(name, value).
				WithDomain(".yandex.ru").
				WithPath("/").
				WithSecure(true).
				WithHTTPOnly(false).
				Do(ctx)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// CreateCall navigates to telemost, clicks "Create video meeting", and extracts the join link.
// It keeps the browser context active (run indefinitely until context is canceled externally).
func (ts *TelemostSession) CreateCall(parentCtx context.Context, cookieStr string) (string, error) {
	ctx, cancel := ts.browser.NewTabContext(parentCtx)
	ts.ctx = ctx
	ts.cancel = cancel

	var joinLink string
	
	// Create channel to wait for execution to reach the point where we extract link
	errCh := make(chan error, 1)
	
	go func() {
		err := chromedp.Run(ctx,
			// 1. Set Cookies
			setCookiesAction(cookieStr),
			// 2. Navigate to Telemost entry page
			chromedp.Navigate("https://telemost.yandex.ru/"),
			// 3. Wait for the page to load and check if logged in (avatar exists) or create call button exists
			chromedp.WaitVisible(`body`, chromedp.ByQuery),
			
			// 4. Find the "Create video meeting" button and click it. 
			// We look for a button containing specific text or matching a class.
			// Actually we can just navigate to https://telemost.yandex.ru/?action=create 
			// which automatically creates a meeting and redirects
			chromedp.Navigate("https://telemost.yandex.ru/?action=create"),
			
			// 5. Wait for redirect to happen. The URL should change to https://telemost.yandex.ru/j/12345
			chromedp.WaitReady(`body`, chromedp.ByQuery),
			chromedp.ActionFunc(func(c context.Context) error {
				for i := 0; i < 30; i++ {
					var currentURL string
					if err := chromedp.Evaluate(`window.location.href`, &currentURL).Do(c); err != nil {
						return err
					}
					if strings.Contains(currentURL, "/j/") {
						joinLink = currentURL
						return nil
					}
					time.Sleep(1 * time.Second)
				}
				return fmt.Errorf("timeout waiting for meeting join link redirect")
			}),
		)
		
		errCh <- err
	}()
	
	// Wait for the join link to be extracted, or for context cancellation / driver error
	select {
	case <-parentCtx.Done():
		return "", parentCtx.Err()
	case err := <-errCh:
		if err != nil {
			return "", err
		}
		return joinLink, nil
	}
}

func (ts *TelemostSession) Close() {
	if ts.cancel != nil {
		ts.cancel()
	}
}
