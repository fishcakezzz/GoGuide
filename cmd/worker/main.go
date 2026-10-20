package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"goguide/internal/api"
	"goguide/internal/trade"
)

func main() {
	db, err := api.OpenDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		runCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		n, err := trade.CloseDue(runCtx, db, 50)
		cancel()
		if err != nil {
			log.Printf("close due orders failed: %v", err)
		} else if n > 0 {
			log.Printf("closed %d due orders", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
