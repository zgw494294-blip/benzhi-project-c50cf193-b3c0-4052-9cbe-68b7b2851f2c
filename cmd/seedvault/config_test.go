package main

import "testing"

func TestResolveAddress(t *testing.T) {
	t.Setenv("PORT", "")
	address, err := resolveAddress("")
	if err != nil || address != defaultAddress {
		t.Fatalf("默认地址错误: %s %v", address, err)
	}
	address, err = resolveAddress("127.0.0.1:19999")
	if err != nil || address != "127.0.0.1:19999" {
		t.Fatalf("显式地址错误: %s %v", address, err)
	}
	if _, err := resolveAddress("0.0.0.0:19081"); err == nil {
		t.Fatal("应拒绝通配监听地址")
	}
}
