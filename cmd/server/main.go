package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"tianque_probe/internal/common"
	"tianque_probe/internal/server/auth"
	"tianque_probe/internal/server/config"
	"tianque_probe/internal/server/gfw"
	"tianque_probe/internal/server/handler"
	"tianque_probe/internal/server/notify"
	"tianque_probe/internal/server/routequality"
	"tianque_probe/internal/server/store"
	"tianque_probe/internal/server/web"
)

func main() {
	configPath := flag.String("config", filepath.Join("data", "config.json"), "配置文件存储路径")
	portOverride := flag.Int("port", 0, "覆盖监听端口")
	version := flag.Bool("v", false, "查看版本号")
	flag.Parse()

	if *version {
		fmt.Printf("TianQue-Probe (AegisProbe) Server %s (Zero Penetration Tunnel)\n", common.Version)
		return
	}

	log.Printf("=================================================================")
	log.Printf("🛡️  天阙·神盾安全探针服务端 (TianQue-Probe Server) 启动中...")
	log.Printf("🛡️  【安全铁律生效】：单向汇报接收、真实 IP 脱敏、零下行控制接口")
	log.Printf("=================================================================")

	// 1. 初始化配置管理器
	cfgMgr, err := config.NewManager(*configPath)
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	cfg := cfgMgr.GetConfig()

	listenPort := cfg.ListenPort
	if *portOverride > 0 {
		listenPort = *portOverride
	}

	log.Printf("[系统配置] 监听端口: %d | 访问模式: %s | 管理员密码: %s", listenPort, cfg.AccessMode, cfg.AdminPassword)

	// 2. 初始化核心组件
	st := store.NewStore()
	st.SetRouteQualityHistoryPath(filepath.Join(filepath.Dir(*configPath), "route_quality_history.json"))
	st.SetNodeStatePath(filepath.Join(filepath.Dir(*configPath), "node_state.json"), cfg.OfflineSec)
	authMgr := auth.NewAuthManager()
	notifier := notify.NewNotifier()
	notifier.UpdateConfig(cfg.TGEnabled, cfg.TGBotToken, cfg.TGChatID, cfg.TGProxy)

	// 将 Telegram 告警注册为节点在线/掉线监听器
	st.AddListener(notifier)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 启动 Telegram 异步发送队列消费循环
	go notifier.Start(ctx)

	// 启动国内连通性 (是否被墙) 异步测活巡检引擎
	gfwChecker := gfw.NewChecker(cfgMgr, st)
	gfwChecker.Start(ctx)
	routeQualityChecker := routequality.NewChecker(cfgMgr, st)
	routeQualityChecker.SetAlertCallback(notifier.Push)
	routeQualityChecker.Start(ctx)

	// 启动后台心跳扫描协程 (每 10 秒巡检一次)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentCfg := cfgMgr.GetConfig()
				threshold := time.Duration(currentCfg.OfflineSec) * time.Second
				st.CheckHeartbeats(currentCfg.Nodes, threshold)
				if err := st.SaveNodeStates(); err != nil {
					log.Printf("保存节点状态快照失败: %v", err)
				}
				onlineNodes := st.GetOnlineNodeIDs(currentCfg.Nodes, threshold)
				renewals, err := cfgMgr.RenewExpiredOnlineNodes(onlineNodes, time.Now())
				if err != nil {
					log.Printf("自动续费更新失败: %v", err)
				}
				for _, event := range renewals {
					log.Printf("自动续费: 节点 %q 到期日从 %s 顺延至 %s (%s)", event.NodeName, event.Previous, event.Next, event.Period)
				}
			}
		}
	}()

	// 启动每日账单到期巡更协程 (每小时比对一次)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		// 启动立即检查一次
		currentCfg := cfgMgr.GetConfig()
		notifier.CheckExpiringNodes(currentCfg.Nodes, currentCfg.WarnDays)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentCfg := cfgMgr.GetConfig()
				notifier.CheckExpiringNodes(currentCfg.Nodes, currentCfg.WarnDays)
			}
		}
	}()

	// 3. 构建 HTTP 路由网关
	mux := http.NewServeMux()
	routerHandler := handler.NewRouterHandler(cfgMgr, st, authMgr, notifier)
	routerHandler.SetGFWChecker(gfwChecker)
	routerHandler.SetRouteQualityChecker(routeQualityChecker)
	routerHandler.RegisterRoutes(mux)

	// 挂载嵌入式单页面前端资源 (彻底根除 404 问题)
	web.RegisterStaticRoutes(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", listenPort),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("🚀 服务端启动完毕！")
		log.Printf("👉 监控大屏: http://127.0.0.1:%d", listenPort)
		log.Printf("👉 管理后台: http://127.0.0.1:%d/admin.html", listenPort)
		log.Printf("👉 初始管理员密码: %s", cfg.AdminPassword)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("正在安全关闭探针主控服务...")
	if err := st.SaveNodeStates(); err != nil {
		log.Printf("保存节点状态快照失败: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	log.Printf("探针主控服务已平稳关闭。")
}
