package main

import (
	"os"

	"github.com/nerdneilsfield/go-translator-agent/internal/cli"
	"github.com/nerdneilsfield/go-translator-agent/internal/logger"
	"go.uber.org/zap"
)

// Version information
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	// 初始化日志
	log := logger.NewLogger(false)
	defer func() {
		_ = log.Sync()
	}()

	// 创建根命令
	rootCmd := cli.NewRootCommand(Version, Commit, BuildDate)

	// 执行命令
	if err := rootCmd.Execute(); err != nil {
		log.Error("执行命令失败", zap.Error(err))
		os.Exit(1)
	}
}
