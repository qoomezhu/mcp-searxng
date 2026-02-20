package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

func ValidateOutboundURL(raw string, allowPrivate bool) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("URL 格式非法: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("仅支持 http/https URL")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return nil, fmt.Errorf("URL 缺少主机名")
	}
	if allowPrivate {
		return parsed, nil
	}

	if isBlockedHostname(host) {
		return nil, fmt.Errorf("禁止访问内网/本地主机地址: %s", host)
	}

	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return nil, fmt.Errorf("禁止访问内网地址: %s", host)
		}
		return parsed, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err == nil {
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return nil, fmt.Errorf("DNS 解析命中内网地址，已阻止访问: %s", host)
			}
		}
	}

	return parsed, nil
}

func isBlockedHostname(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "0.0.0.0" || host == "::1" {
		return true
	}
	if strings.HasSuffix(host, ".local") {
		return true
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsMulticast() || addr.IsUnspecified() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return true
	}

	if addr.Is4() {
		v := addr.As4()
		// Carrier-grade NAT: 100.64.0.0/10
		if v[0] == 100 && v[1] >= 64 && v[1] <= 127 {
			return true
		}
		// Benchmark + reserved blocks
		if v[0] == 198 && (v[1] == 18 || v[1] == 19) {
			return true
		}
	}

	return false
}
