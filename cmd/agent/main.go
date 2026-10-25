package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tianque_probe/internal/agent/reporter"
	"tianque_probe/internal/common"
)

func main() {
	serverURL := flag.String("server", "http://127.0.0.1:8989", "探针服务端地址 (如 http://master-ip:8989)")
	nodeID := flag.String("id", "", "节点唯一标识 (如 hk-relay-01)")
	token := flag.String("token", "", "接入认证 Token")
	intervalSec := flag.Int("interval", 2, "数据上报间隔秒数")
	version := flag.Bool("v", false, "查看版本号")
	flag.Parse()

	if *version {
		fmt.Printf("TianQue-Probe (AegisProbe) Agent %s (Pure Push-Only)\n", common.Version)
		return
	}

	if *nodeID == "" || *token == "" {
		fmt.Println("错误: 必须指定 -id 和 -token 参数！")
		fmt.Println("示例: aegis_agent -server=http://1.2.3.4:8989 -id=hk-01 -token=your_token")
		flag.Usage()
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rep := reporter.NewReporter(reporter.Config{
		ServerURL: *serverURL,
		NodeID:    *nodeID,
		Token:     *token,
		Interval:  time.Duration(*intervalSec) * time.Second,
	})

	rep.Start(ctx)
}
