//go:build windows

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/windows/svc"
)

const windowsServiceName = "WeeklineHost"

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
	if hasArgument("--service") {
		if err := svc.Run(windowsServiceName, &weeklineService{}); err != nil {
			log.Fatal(err)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

type weeklineService struct{}

func (service *weeklineService) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- run(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case err := <-errCh:
			if err != nil {
				log.Printf("Weekline host stopped: %v", err)
				return true, 1
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				status <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-errCh; err != nil {
					log.Printf("Weekline host shutdown: %v", err)
				}
				return false, 0
			}
		}
	}
}
