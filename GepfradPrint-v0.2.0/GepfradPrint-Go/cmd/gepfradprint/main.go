package main

import (
	"context"
	"fmt"
	"github.com/gepfrad/gepfradprint/internal/api"
	"github.com/gepfrad/gepfradprint/internal/store"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

var version = "dev"

func main() {
	st, e := store.New()
	if e != nil {
		log.Fatal(e)
	}
	srv := api.New(st)
	h := &http.Server{Addr: "127.0.0.1:17842", Handler: srv, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		fmt.Printf("Gepfrad Print %s: http://127.0.0.1:17842/\n", version)
		if e := h.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Fatal(e)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.Shutdown(shutdownCtx)
}
