package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"dingtalk-bridge/internal/bridge"
	"dingtalk-bridge/internal/config"
	"dingtalk-bridge/internal/dingtalk"
	"dingtalk-bridge/internal/logger"
	"dingtalk-bridge/internal/opencode"
)

func main() {
	configPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	_ = configPath

	if err := logger.Init(cfg.LogLevel, cfg.GetExpandedLogFilePath()); err != nil {
		fmt.Fprintf(os.Stderr, "Logger initialization error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting dingtalk-bridge...")
	logger.Infof("Configuration: mode=%s, host=%s, port=%d", cfg.BridgeMode, cfg.BridgeHost, cfg.BridgePort)

	// Load user whitelist if configured
	if cfg.DingTalkUserWhitelistPath != "" {
		userWhitelist, err := config.LoadUserWhitelist(cfg.DingTalkUserWhitelistPath)
		if err != nil {
			logger.Warnf("Failed to load user whitelist: %v (continuing without whitelist)", err)
		} else {
			cfg.UserWhitelist = userWhitelist
			logger.Infof("User whitelist loaded: enabled=%v, users=%d", userWhitelist.Enabled, len(userWhitelist.Users))
		}
	}

	opencodeClient := opencode.NewServerClient(
		cfg.OpenCodeServerURL,
		cfg.OpenCodeServerUsername,
		cfg.OpenCodeServerPassword,
		cfg.OpenCodeProviderID,
		cfg.OpenCodeModelID,
		cfg.OpenCodeAgent,
	)

	if err := opencodeClient.HealthCheck(context.Background()); err != nil {
		logger.Fatalf("OpenCode server health check failed: %v", err)
	}
	logger.Info("OpenCode server connection verified")

	cardClient, err := dingtalk.NewCardClient(cfg.DingTalkClientID, cfg.DingTalkClientSecret)
	if err != nil {
		logger.Fatalf("Failed to create DingTalk card client: %v", err)
	}

	dingClient := dingtalk.NewClient(cfg.DingTalkClientID, cfg.DingTalkClientSecret)

	router := bridge.NewRouter(cfg, dingClient, cardClient, opencodeClient)

	sessionStorePath := cfg.GetExpandedSessionStorePath()
	if err := router.GetSessionStore().LoadFromFile(sessionStorePath); err != nil {
		logger.Warnf("Failed to load sessions from %s: %v (starting with empty store)", sessionStorePath, err)
	} else {
		logger.Infof("Loaded %d sessions from %s", router.GetSessionStore().Count(), sessionStorePath)
	}

	dingClient.SetMessageCallback(func(ctx context.Context, msg *dingtalk.ReceivedMessage) error {
		return router.HandleMessage(ctx, msg)
	})

	dingClient.SetCardActionCallback(func(ctx context.Context, sessionKey, action string) error {
		return router.HandleCardAction(ctx, sessionKey, action)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Received shutdown signal, gracefully stopping...")

		if err := router.GetSessionStore().SaveToFile(sessionStorePath); err != nil {
			logger.Warnf("Failed to save sessions to %s: %v", sessionStorePath, err)
		} else {
			logger.Infof("Saved %d sessions to %s", router.GetSessionStore().Count(), sessionStorePath)
		}

		cancel()
	}()

	if err := dingClient.Start(ctx); err != nil {
		logger.Fatalf("Failed to start DingTalk client: %v", err)
	}

	<-ctx.Done()
	logger.Info("dingtalk-bridge stopped")
}
