package utils

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"time"
)

// GenerateID generates a random hex ID
func GenerateID(length int) string {
	bytes := make([]byte, length/2)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GetLocalIP returns the local IP address
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "unknown"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "unknown"
}

// RetryWithBackoff retries a function with exponential backoff
func RetryWithBackoff(fn func() error, maxRetries int, initialDelay time.Duration) error {
	delay := initialDelay
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := fn(); err != nil {
			lastErr = err
			time.Sleep(delay)
			delay *= 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			continue
		}
		return nil
	}
	return lastErr
}

// MinInt returns the minimum of two integers
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// MaxInt returns the maximum of two integers
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

