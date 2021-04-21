package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"regexp"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// GenerateRandomString generates random string of requested length
func GenerateRandomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
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
	return fmt.Errorf("not a local IP: %s", ip)
}

// NormalizeNewlines normalizes \r\n (windows) and \r (mac)
// into \n (unix)
func NormalizeNewlines(d []byte) []byte {
	// replace CR LF \r\n (windows) with LF \n (unix)
	d = bytes.Replace(d, []byte{13, 10}, []byte{10}, -1)
	// replace CF \r (mac) with LF \n (unix)
	d = bytes.Replace(d, []byte{13}, []byte{10}, -1)
	return d
}
