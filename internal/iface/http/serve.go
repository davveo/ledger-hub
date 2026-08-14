package httpserver

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

func Serve(addr string, handler http.Handler, readTO, writeTO, shutdownTO time.Duration, log *zap.Logger) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  readTO,
		WriteTimeout: writeTO,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("http listen", zap.String("addr", addr))
		errCh <- srv.ListenAndServe()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-sig:
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTO)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
