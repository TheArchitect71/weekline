//go:build !windows

package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
)

func main() {
	if err := loadRuntimeConfig(); err != nil {
		log.Fatal(err)
	}
	if hasArgument("--bootstrap-manager") {
		if err := bootstrapManager(context.Background()); err != nil {
			log.Fatal(err)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}
