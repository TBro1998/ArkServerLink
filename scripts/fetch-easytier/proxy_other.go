//go:build !windows

package main

import "net/url"

func systemProxy() *url.URL { return nil }
