package cli

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/hazzardr/sbom-cli/internal/api"
)

const shutdownTimeout = 10 * time.Second

var serveAddr string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve ingest and query over HTTP (used by the performance tests)",
	Long: `Serve ingest and query over HTTP (used by the performance tests).

  POST /sboms                                     body: SBOM document
  GET  /components?component=<name>[&version=<version>]
  GET  /components?license=<license>`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		s, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer s.Close()

		srv := &http.Server{
			Addr:              serveAddr,
			Handler:           api.New(s),
			ReadHeaderTimeout: 5 * time.Second,
		}
		errc := make(chan error, 1)
		go func() { errc <- srv.ListenAndServe() }()
		slog.Info("serving", "addr", serveAddr, "db", dbPath)

		select {
		case err := <-errc:
			return err
		case <-ctx.Done():
		}
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", "127.0.0.1:8080", "listen address")
	rootCmd.AddCommand(serveCmd)
}
