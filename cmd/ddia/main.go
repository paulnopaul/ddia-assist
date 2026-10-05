// Command ddia is the single binary behind ddia-assist (see docs/spec/00-overview.md).
//
//	ddia serve   web tracker and MCP over streamable HTTP at /mcp
//	ddia mcp     MCP over stdio, started by Claude Desktop via docker exec (DEP-3)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/paulnopaul/ddia-assist/internal/mcpserver"
	"github.com/paulnopaul/ddia-assist/internal/store"
	"github.com/paulnopaul/ddia-assist/internal/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "serve":
		err = runServe(ctx, args)
	case "mcp":
		err = runMCP(ctx, args)
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ddia:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ddia <serve|mcp|version> [flags]")
}

func dataDirFlag(fs *flag.FlagSet) *string {
	def := os.Getenv("DDIA_DATA_DIR")
	if def == "" {
		def = "/data"
	}
	return fs.String("data", def, "data directory holding ddia.db (env DDIA_DATA_DIR)")
}

func openStore(ctx context.Context, dataDir string) (*store.Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	return store.Open(ctx, filepath.Join(dataDir, "ddia.db"))
}

func runServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dataDir := dataDirFlag(fs)
	addr := fs.String("addr", ":8080", "listen address")
	_ = fs.Parse(args)

	st, err := openStore(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := mcpserver.New(st, version)
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	mux.Handle("/", web.Handler(st, version, *dataDir))

	hs := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(shutdownCtx)
	}()
	slog.Info("serving", "addr", *addr, "data", *dataDir, "version", version)
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runMCP(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	dataDir := dataDirFlag(fs)
	_ = fs.Parse(args)

	// stdout carries the MCP protocol, so logs must go to stderr.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	st, err := openStore(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	return mcpserver.New(st, version).Run(ctx, &mcp.StdioTransport{})
}
