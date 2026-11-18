package main

import (
	"net/url"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// systemProxy 读取「设置 → 网络 → 代理」里的手动代理。
func systemProxy() *url.URL {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	if enabled, _, err := k.GetIntegerValue("ProxyEnable"); err != nil || enabled == 0 {
		return nil
	}
	server, _, err := k.GetStringValue("ProxyServer")
	if err != nil || server == "" {
		return nil
	}
	// 也可能是 "http=host:port;https=host:port" 的分协议写法
	if strings.Contains(server, "=") {
		for part := range strings.SplitSeq(server, ";") {
			if v, ok := strings.CutPrefix(part, "https="); ok {
				server = v
				break
			}
		}
	}
	if !strings.Contains(server, "://") {
		server = "http://" + server
	}
	u, err := url.Parse(server)
	if err != nil {
		return nil
	}
	return u
}
