package collector

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// refreshPublicAddresses updates cached egress addresses from outside the NAT.
// It is called asynchronously and at a low cadence, not on every agent report.
func refreshPublicAddresses(onIP func(string), onCountry func(string)) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 4 * time.Second}
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ip.sb/geoip", nil); err == nil {
		resp, err := client.Do(req)
		if err == nil {
			var result struct {
				IP          string `json:"ip"`
				CountryCode string `json:"country_code"`
			}
			if json.NewDecoder(io.LimitReader(resp.Body, 16*1024)).Decode(&result) == nil {
				if ip := net.ParseIP(strings.TrimSpace(result.IP)); ip != nil {
					onIP(ip.String())
				}
				if result.CountryCode != "" {
					onCountry(strings.ToUpper(strings.TrimSpace(result.CountryCode)))
				}
			}
			resp.Body.Close()
		}
	}
	if ip := queryPublicIP(ctx, []string{"https://api-ipv4.ip.sb/ip", "https://v4.ident.me", "https://api4.ipify.org", "http://ipv4.icanhazip.com"}, true); ip != "" {
		onIP(ip)
	}
	if ip := queryPublicIP(ctx, []string{"https://api-ipv6.ip.sb/ip", "https://v6.ident.me", "https://api6.ipify.org", "http://ipv6.icanhazip.com"}, false); ip != "" {
		onIP(ip)
	}
}

func queryPublicIP(ctx context.Context, endpoints []string, ipv4 bool) string {
	client := &http.Client{Timeout: 3 * time.Second}
	for _, endpoint := range endpoints {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if reqErr != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 128))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(string(body)))
		if ip == nil {
			continue
		}
		if ipv4 && ip.To4() != nil {
			return ip.To4().String()
		}
		if !ipv4 && ip.To4() == nil && ip.To16() != nil {
			return ip.String()
		}
	}
	return ""
}
