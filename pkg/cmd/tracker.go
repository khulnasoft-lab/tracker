package cmd

import (
	"context"
	"os"
	"strconv"
	"syscall"

	"github.com/khulnasoft-lab/tracker/pkg/cmd/printer"
	"github.com/khulnasoft-lab/tracker/pkg/config"
	tracker "github.com/khulnasoft-lab/tracker/pkg/ebpf"
	"github.com/khulnasoft-lab/tracker/pkg/errfmt"
	"github.com/khulnasoft-lab/tracker/pkg/logger"
	"github.com/khulnasoft-lab/tracker/pkg/server/grpc"
	"github.com/khulnasoft-lab/tracker/pkg/server/http"
	"github.com/khulnasoft-lab/tracker/pkg/utils"
)

type Runner struct {
	TrackerConfig config.Config
	Printer      printer.EventPrinter
	HTTPServer   *http.Server
	GRPCServer   *grpc.Server
}

func (r Runner) Run(ctx context.Context) error {
	// Create Tracker Singleton

	t, err := tracker.New(r.TrackerConfig)
	if err != nil {
		return errfmt.Errorf("error creating Tracker: %v", err)
	}

	// Readiness Callback: Tracker is ready to receive events
	t.AddReadyCallback(
		func(ctx context.Context) {
			logger.Debugw("Tracker is ready callback")
			if r.HTTPServer != nil {
				if r.HTTPServer.MetricsEndpointEnabled() {
					r.TrackerConfig.MetricsEnabled = true // TODO: is this needed ?
					if err := t.Stats().RegisterPrometheus(); err != nil {
						logger.Errorw("Registering prometheus metrics", "error", err)
					}
				}
				go r.HTTPServer.Start(ctx)
			}

			// start server if one is configured
			if r.GRPCServer != nil {
				go r.GRPCServer.Start(ctx, t)
			}
		},
	)

	// Initialize tracker

	err = t.Init(ctx)
	if err != nil {
		return errfmt.Errorf("error initializing Tracker: %v", err)
	}

	// Manage PID file

	if err := writePidFile(t.OutDir); err != nil {
		return errfmt.WrapError(err)
	}
	defer func() {
		if err := removePidFile(t.OutDir); err != nil {
			logger.Warnw("error removing pid file", "error", err)
		}
	}()

	stream := t.SubscribeAll()
	defer t.Unsubscribe(stream)

	// Preeamble

	r.Printer.Preamble()

	// Start event channel reception

	go func() {
		for {
			select {
			case event := <-stream.ReceiveEvents():
				r.Printer.Print(event)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Blocks (until ctx is Done)
	err = t.Run(ctx)

	// Drain remaininig channel events (sent during shutdown),
	// the channel is closed by the tracker when it's done
	for event := range stream.ReceiveEvents() {
		r.Printer.Print(event)
	}

	stats := t.Stats()
	r.Printer.Epilogue(*stats)
	r.Printer.Close()

	return err
}

func GetContainerMode(cfg config.Config) config.ContainerMode {
	containerMode := config.ContainerModeDisabled

	for p := range cfg.Policies.Map() {
		if p.ContainerFilterEnabled() {
			// Container Enrichment is enabled by default ...
			containerMode = config.ContainerModeEnriched
			if cfg.NoContainersEnrich {
				// ... but might be disabled as a safeguard measure.
				containerMode = config.ContainerModeEnabled
			}

			break
		}
	}

	return containerMode
}

const pidFileName = "tracker.pid"

// Initialize PID file
func writePidFile(dir *os.File) error {
	pidFile, err := utils.OpenAt(dir, pidFileName, syscall.O_WRONLY|syscall.O_CREAT, 0640)
	if err != nil {
		return errfmt.Errorf("error creating readiness file: %v", err)
	}

	_, err = pidFile.Write([]byte(strconv.Itoa(os.Getpid()) + "\n"))
	if err != nil {
		return errfmt.Errorf("error writing to readiness file: %v", err)
	}

	return nil
}

// Remove PID file
func removePidFile(dir *os.File) error {
	if err := utils.RemoveAt(dir, pidFileName, 0); err != nil {
		return errfmt.Errorf("%v", err)
	}

	return nil
}
