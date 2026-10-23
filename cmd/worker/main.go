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
		replanCtx, replanCancel := context.WithTimeout(ctx, 40*time.Second)
		changed, replanErr := api.ReconcileProposals(replanCtx, db, 25)
		replanCancel()
		if replanErr != nil {
			log.Printf("guide replan failed: %v", replanErr)
		} else if changed > 0 {
			log.Printf("replanned %d guide sessions", changed)
		}
		watchCtx, watchCancel := context.WithTimeout(ctx, 4*time.Second)
		alerts, watchErr := api.CheckPriceWatches(watchCtx, db)
		watchCancel()
		if watchErr != nil {
			log.Printf("price watch check failed: %v", watchErr)
		} else if alerts > 0 {
			log.Printf("created %d simulated price watch events", alerts)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
