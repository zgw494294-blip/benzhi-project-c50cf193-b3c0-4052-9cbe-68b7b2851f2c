package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"seedvault/internal/persistence"
	webapp "seedvault/internal/web"
	"seedvault/internal/workflow"
)

func main() {
	addressFlag := flag.String("addr", "", "回环监听地址，例如 127.0.0.1:19081")
	dataFlag := flag.String("data", ".seedvault-data", "本地事件账本和投影目录")
	selfcheck := flag.Bool("selfcheck", false, "启动有界 HTTP 服务并执行端到端自检")
	flag.Parse()
	address, err := resolveAddress(*addressFlag)
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}
	dataDir := *dataFlag
	cleanup := func() {}
	if *selfcheck {
		dataDir, err = os.MkdirTemp("", "seedvault-selfcheck-")
		if err != nil {
			log.Fatalf("创建自检目录: %v", err)
		}
		cleanup = func() { _ = os.RemoveAll(dataDir) }
	}
	defer cleanup()
	store, err := persistence.Open(dataDir)
	if err != nil {
		log.Fatalf("打开持久层: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("关闭持久层: %v", err)
		}
	}()
	service, err := workflow.NewService(store)
	if err != nil {
		log.Fatalf("恢复工作流: %v", err)
	}
	handler := webapp.New(service).Routes()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("监听 %s: %v", address, err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	if *selfcheck {
		if err := runSelfcheck(server, listener); err != nil {
			log.Fatalf("自检失败: %v", err)
		}
		fmt.Printf("selfcheck passed: %s\n", listener.Addr().String())
		return
	}
	go func() {
		log.Printf("种源活力入库核验台已监听 http://%s", listener.Addr().String())
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP 服务异常: %v", err)
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	context, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(context); err != nil {
		log.Printf("优雅关停失败: %v", err)
	}
}
