package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/packalares/packalares/internal/hfdownload"
)

func main() {
	cfg := hfdownload.DefaultConfig()

	// Required.
	cfg.Repo = os.Getenv("HF_REPO")
	if cfg.Repo == "" {
		fmt.Fprintln(os.Stderr, "HF_REPO is required")
		os.Exit(1)
	}

	// Optional overrides.
	if v := os.Getenv("HF_REF"); v != "" {
		cfg.Ref = v
	}
	if v := os.Getenv("HF_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("HF_ENDPOINT"); v != "" {
		cfg.Endpoint = v
	}
	if v := os.Getenv("OUTPUT_DIR"); v != "" {
		cfg.OutputDir = v
	}
	if v := os.Getenv("DONE_NAME"); v != "" {
		cfg.DoneName = v
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	progress := hfdownload.NewProgressReporter()

	// Download tiktoken files if configured.
	tiktokenFiles := hfdownload.ParseTiktokenFiles(os.Getenv("TIKTOKEN_FILES"))
	if len(tiktokenFiles) > 0 {
		tiktokenDir := os.Getenv("TIKTOKEN_DIR")
		if tiktokenDir == "" {
			tiktokenDir = "/data/tiktoken"
		}
		tikCfg := hfdownload.TiktokenConfig{
			Files: tiktokenFiles,
			Dir:   tiktokenDir,
		}
		if err := hfdownload.DownloadTiktoken(ctx, tikCfg, progress); err != nil {
			fmt.Fprintf(os.Stderr, "tiktoken download failed: %v\n", err)
			os.Exit(1)
		}
	}

	// Download model files.
	client := hfdownload.NewClient(cfg, progress)
	if err := client.DownloadAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "download failed: %v\n", err)
		os.Exit(1)
	}
}
