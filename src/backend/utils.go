package main

import (
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"strings"
	"time"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var src = rand.NewSource(time.Now().UnixNano())

// GenerateRandomString generates random string of requested length
func GenerateRandomString(n int) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

// Copied from https://github.com/acarl005/stripansi/blob/master/stripansi.go
const colorEscapeCodes = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var colorEscapeCodesRE = regexp.MustCompile(colorEscapeCodes)

// StripColor removes color escape codes from string
func StripColor(str string) string {
	return colorEscapeCodesRE.ReplaceAllString(str, "")
}

// EnsureLocalIP returns error if IP address is not local
func EnsureLocalIP(ip string) error {
	ipObj := net.ParseIP(ip)

	if ipObj.IsLoopback() {
		return nil
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}

	// Create list of local IP addresses
	localAddrs := []net.IP{}
	for _, iface := range interfaces {
		if strings.Contains(iface.Flags.String(), "up") {
			addrs, err := iface.Addrs()
			if err != nil {
				return err
			}
			for _, addr := range addrs {
				ip, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					return err
				}
				localAddrs = append(localAddrs, ip)
			}
		}
	}

	// Verify if ip matched any
	for _, item := range localAddrs {
		if ipObj.Equal(item) {
			return nil
		}
	}
	return fmt.Errorf("Not a local IP: %s", ip)
}
