package osc

import (
	"context"
	"testing"
)

func TestBrowserClosesBothServiceBrowses(t *testing.T) {
	browser, err := NewBrowser(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := browser.Start(); err != nil {
		t.Fatal(err)
	}
	browser.Close()
	if _, ok := <-browser.Updates(); ok {
		t.Fatal("browser updates remained open after close")
	}
}
