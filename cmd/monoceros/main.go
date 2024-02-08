// Command monoceros is a CLI for a multi-application controller
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/version"
	"github.com/reddit/monoceros/internal/admin"
	"github.com/reddit/monoceros/internal/config"
	"github.com/reddit/monoceros/internal/supervisor"
	"github.com/reddit/monoceros/internal/worker"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

const AppName = "monoceros"

func mainCore(config *config.MonocerosConfig, logger *zap.Logger) error {
	logger.Info("Monoceros started")
	supervisorCtx := context.Background()
	supervisorCtx, supervisorCancel := context.WithCancel(supervisorCtx)

	adminCtx := context.Background()
	adminCtx, adminCancel := context.WithCancel(adminCtx)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(ch)
	}()

	go func() {
		select {
		case <-ch:
			logger.Info("Actually caught signal")
			supervisorCancel()
			adminCancel()
		case <-supervisorCtx.Done():
		case <-adminCtx.Done():
		}
	}()

	admni := admin.NewAdminInterface(logger)
	supv, err := supervisor.NewSupervisor(config.SupervisorConfig, worker.NewProdWorker, logger)
	if err != nil {
		logger.Fatal("Initialization error", zap.String("function", "supervisor.NewSupervisor"), zap.Error(err))
	}
	healthCheck := admin.HealthHandlerFunc(supv.IsHealthy)

	var wg sync.WaitGroup

	adminErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		adminErr <- admni.Run(adminCtx, config.AdminConfig, healthCheck)
	}()

	supervisorErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		supervisorErr <- supv.Run(supervisorCtx, supervisorCancel)
	}()

	var firstError error
	select {
	case err := <-adminErr:
		if firstError == nil {
			firstError = err
		}
		if err != nil {
			logger.Error("Initialization error", zap.String("function", "admin.Run"), zap.Error(err))
		}
		supervisorCancel()
	case err := <-supervisorErr:
		if firstError == nil {
			firstError = err
		}
		if err != nil {
			logger.Error("Initialization error", zap.String("function", "supervisor.Run"), zap.Error(err))
		}
		adminCancel()
	}

	wg.Wait()
	return firstError
}

func main() {
	prometheus.MustRegister(version.NewCollector(AppName))
	rand.Seed(time.Now().UnixNano())

	app := &cli.App{
		Name:        AppName,
		Usage:       "A Multi-process Application Controller",
		Description: "monoceros makes it easy to run, keep alive and terminate multiple copies of single server.",
		Version:     fmt.Sprintf("\n Info: %s\n Build Context: %s", version.Info(), version.BuildContext()),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "config",
				Aliases:  []string{"c"},
				Value:    "config.yaml",
				Usage:    "Path to the config file",
				Required: true,
			},
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
				Usage:   "Turn development mode debugging. Not recommended for production environment.",
			},
			&cli.BoolFlag{
				Name:    "validate",
				Aliases: []string{"k"},
				Usage:   "Only validate the provided config. This is useful for CI validation.",
			},
		},
		Action: func(c *cli.Context) error {
			var zCfg zap.Config
			if c.Bool("debug") {
				zCfg = zap.NewDevelopmentConfig()
			} else {
				zCfg = zap.NewProductionConfig()
			}
			zCfg.DisableCaller = true
			zCfg.DisableStacktrace = true
			logger, err := zCfg.Build()
			if err != nil {
				panic(err)
			}

			mCfg, err := config.LoadConf(c.String("config"))
			if err != nil {
				logger.Fatal("Initialization error", zap.String("function", "loadConf"), zap.Error(err))
				return err
			}

			if c.Bool("validate") {
				// if we reached here it means that the config is all correct
				logger.Info("Config provided is valid")
				return nil
			}

			return mainCore(mCfg, logger)
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		os.Exit(1)
	}
}
