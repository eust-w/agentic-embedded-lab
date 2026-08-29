package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chromedp/chromedp"
)

type Controller struct {
	Executable  string
	ProfilePath string
	Permissions *PermissionStore
	mu          sync.Mutex
	allocator   context.Context
	allocCancel context.CancelFunc
	context     context.Context
	cancel      context.CancelFunc
}

func (c *Controller) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.context != nil {
		return nil
	}
	if !filepath.IsAbs(c.Executable) || !filepath.IsAbs(c.ProfilePath) {
		return errors.New("Chromium executable and profile path must be absolute")
	}
	info, err := os.Stat(c.Executable)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("bundled Chromium executable is unavailable")
	}
	if err := os.MkdirAll(c.ProfilePath, 0o700); err != nil {
		return err
	}
	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(c.Executable), chromedp.UserDataDir(c.ProfilePath),
		chromedp.Flag("headless", false), chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-background-networking", true), chromedp.Flag("no-first-run", true))
	allocator, allocCancel := chromedp.NewExecAllocator(ctx, options...)
	browserContext, cancel := chromedp.NewContext(allocator)
	c.allocator, c.allocCancel, c.context, c.cancel = allocator, allocCancel, browserContext, cancel
	return chromedp.Run(browserContext)
}

func (c *Controller) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	if c.allocCancel != nil {
		c.allocCancel()
	}
	c.allocator, c.context, c.cancel, c.allocCancel = nil, nil, nil, nil
}

func (c *Controller) Navigate(ctx context.Context, target string) error {
	if c.Permissions == nil {
		return errors.New("browser permission store is required")
	}
	decision, err := c.Permissions.Site(ctx, target)
	if err != nil {
		return err
	}
	if decision != DecisionAllow {
		return fmt.Errorf("site permission is %s", decision)
	}
	return c.run(ctx, chromedp.Navigate(target))
}

func (c *Controller) DOM(ctx context.Context) (string, error) {
	var html string
	err := c.run(ctx, chromedp.OuterHTML("html", &html, chromedp.ByQuery))
	return html, err
}

func (c *Controller) Screenshot(ctx context.Context) ([]byte, error) {
	var payload []byte
	err := c.run(ctx, chromedp.CaptureScreenshot(&payload))
	return payload, err
}

func (c *Controller) Click(ctx context.Context, selector string) error {
	if strings.TrimSpace(selector) == "" {
		return errors.New("selector is required")
	}
	return c.run(ctx, chromedp.Click(selector, chromedp.ByQuery))
}

func (c *Controller) Type(ctx context.Context, selector, text string) error {
	if selector == "" {
		return errors.New("selector is required")
	}
	return c.run(ctx, chromedp.SendKeys(selector, text, chromedp.ByQuery))
}

func (c *Controller) run(ctx context.Context, actions ...chromedp.Action) error {
	c.mu.Lock()
	browserContext := c.context
	c.mu.Unlock()
	if browserContext == nil {
		return errors.New("browser is not running")
	}
	merged, cancel := context.WithCancel(browserContext)
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-merged.Done():
		}
	}()
	return chromedp.Run(merged, actions...)
}
