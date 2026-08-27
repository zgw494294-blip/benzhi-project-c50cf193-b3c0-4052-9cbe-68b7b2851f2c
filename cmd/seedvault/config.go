package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

func resolveAddress(flagValue string) (string, error) {
	address := strings.TrimSpace(flagValue)
	if address == "" {
		if rawPort := strings.TrimSpace(os.Getenv("PORT")); rawPort != "" {
			port, err := strconv.Atoi(rawPort)
			if err != nil || port < 1 || port > 65535 {
				return "", errors.New("PORT 必须是 1 到 65535 的端口号")
			}
			address = fmt.Sprintf("127.0.0.1:%d", port)
		} else {
			address = defaultAddress
		}
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("监听地址必须是 host:port: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("监听端口无效")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("监听地址必须使用明确的回环 IP，例如 127.0.0.1:19081")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}
