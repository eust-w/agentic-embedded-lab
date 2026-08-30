package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

type ConsoleEntry struct {
	Level     string    `json:"level"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

type NetworkEntry struct {
	Method    string    `json:"method"`
	URL       string    `json:"url"`
	Status    int64     `json:"status"`
	MIMEType  string    `json:"mime_type"`
	Timestamp time.Time `json:"timestamp"`
}

type Status struct {
	Running    bool   `json:"running"`
	Executable string `json:"executable"`
	URL        string `json:"url,omitempty"`
	Title      string `json:"title,omitempty"`
}

type Controller struct {
	Executable  string
	ProfilePath string
	Permissions *PermissionStore
	mu          sync.Mutex
	allocator   context.Context
	allocCancel context.CancelFunc
	context     context.Context
	cancel      context.CancelFunc
	currentURL  string
	title       string
	console     []ConsoleEntry
	network     []NetworkEntry
}

func (c *Controller) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.context != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
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
	chromedp.ListenTarget(browserContext, func(event any) {
		c.mu.Lock()
		defer c.mu.Unlock()
		switch value := event.(type) {
		case *cdpruntime.EventConsoleAPICalled:
			parts := make([]string, 0, len(value.Args))
			for _, argument := range value.Args {
				if argument.Value != nil {
					parts = append(parts, string(argument.Value))
				} else if argument.Description != "" {
					parts = append(parts, argument.Description)
				}
			}
			c.console = appendBounded(c.console, ConsoleEntry{Level: string(value.Type), Text: strings.Join(parts, " "), Timestamp: time.Now().UTC()}, 500)
		case *cdpruntime.EventExceptionThrown:
			c.console = appendBounded(c.console, ConsoleEntry{Level: "error", Text: value.ExceptionDetails.Text, Timestamp: time.Now().UTC()}, 500)
		case *network.EventResponseReceived:
			c.network = appendBounded(c.network, NetworkEntry{URL: value.Response.URL, Status: value.Response.Status, MIMEType: value.Response.MimeType, Timestamp: time.Now().UTC()}, 1000)
		}
	})
	c.mu.Lock()
	c.allocator, c.allocCancel, c.context, c.cancel = allocator, allocCancel, browserContext, cancel
	c.mu.Unlock()
	if err := chromedp.Run(browserContext, network.Enable(), cdpruntime.Enable()); err != nil {
		c.Stop()
		return err
	}
	return nil
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
	c.currentURL, c.title = "", ""
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
	var title, location string
	if err := c.run(ctx, chromedp.Navigate(target), chromedp.Location(&location), chromedp.Title(&title)); err != nil {
		return err
	}
	c.mu.Lock()
	c.currentURL, c.title = location, title
	c.mu.Unlock()
	return nil
}

func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Status{Running: c.context != nil, Executable: c.Executable, URL: c.currentURL, Title: c.title}
}

func (c *Controller) Console(after int) []ConsoleEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if after < 0 {
		after = 0
	}
	if after > len(c.console) {
		after = len(c.console)
	}
	return append([]ConsoleEntry(nil), c.console[after:]...)
}

func (c *Controller) Network(after int) []NetworkEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if after < 0 {
		after = 0
	}
	if after > len(c.network) {
		after = len(c.network)
	}
	return append([]NetworkEntry(nil), c.network[after:]...)
}

func (c *Controller) DOM(ctx context.Context) (string, error) {
	var html string
	err := c.run(ctx, chromedp.OuterHTML("html", &html, chromedp.ByQuery))
	return html, err
}

func appendBounded[T any](values []T, value T, limit int) []T {
	values = append(values, value)
	if len(values) > limit {
		values = append([]T(nil), values[len(values)-limit:]...)
	}
	return values
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
	var inputType string
	var exists bool
	if err := c.run(ctx, chromedp.AttributeValue(selector, "type", &inputType, &exists, chromedp.ByQuery)); err != nil {
		return err
	}
	if exists && strings.EqualFold(inputType, "password") {
		return errors.New("typing into password fields is prohibited")
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
	merged, cancel := context.WithTimeout(browserContext, 30*time.Second)
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
