package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/open-policy-agent/opa/compile"
	"github.com/urfave/cli/v2"
	"kernel.org/pub/linux/libs/security/libcap/cap"

	"github.com/khulnasoft-lab/tracker/pkg/capabilities"
	"github.com/khulnasoft-lab/tracker/pkg/cmd/flags/server"
	"github.com/khulnasoft-lab/tracker/pkg/logger"
	"github.com/khulnasoft-lab/tracker/pkg/signatures/engine"
	"github.com/khulnasoft-lab/tracker/pkg/signatures/signature"
	"github.com/khulnasoft-lab/tracker/types/detect"
)

const (
	signatureBufferFlag = "sig-buffer"
)

func main() {
	app := &cli.App{
		Name:  "tracker-rules",
		Usage: "A rule engine for Runtime Security",
		Action: func(c *cli.Context) error {
			// Logger Setup
			logger.Init(logger.NewDefaultLoggingConfig())

			// Capabilities command line flags

			if c.NumFlags() == 0 {
				if err := cli.ShowAppHelp(c); err != nil {
					logger.Errorw("Failed to show app help", "error", err)
				}
				return errors.New("no flags specified")
			}

			var target string
			switch strings.ToLower(c.String("rego-runtime-target")) {
			case "wasm":
				return errors.New("target unsupported: wasm")
			case "rego":
				target = compile.TargetRego
			default:
				return fmt.Errorf("invalid target specified: %s", strings.ToLower(c.String("rego-runtime-target")))
			}

			var rulesDir []string
			if c.String("rules-dir") != "" {
				rulesDir = []string{c.String("rules-dir")}
			}

			sigs, err := signature.Find(
				target,
				c.Bool("rego-partial-eval"),
				rulesDir,
				c.StringSlice("rules"),
				c.Bool("rego-aio"),
			)
			if err != nil {
				return err
			}

			// can't drop privileges before this point due to signature.Find(),
			// orelse we would have to raise capabilities in Find() and it can't
			// be done in the single binary case (capabilities initialization
			// happens after Find() is called) in that case.

			bypass := c.Bool("allcaps") || !isRoot()
			err = capabilities.Initialize(bypass)
			if err != nil {
				return err
			}

			var loadedSigIDs []string
			err = capabilities.GetInstance().Specific(
				func() error {
					for _, s := range sigs {
						m, err := s.GetMetadata()
						if err != nil {
							logger.Errorw("Failed to load signature", "error", err)
							continue
						}
						loadedSigIDs = append(loadedSigIDs, m.ID)
					}
					return nil
				},
				cap.DAC_OVERRIDE,
			)
			if err != nil {
				logger.Errorw("Requested capabilities", "error", err)
			}

			if c.Bool("list-events") {
				listEvents(os.Stdout, sigs)
				return nil
			}

			logger.Infow("Signatures loaded", "total", len(loadedSigIDs), "signatures", loadedSigIDs)

			if c.Bool("list") {
				listSigs(os.Stdout, sigs)
				return nil
			}

			var inputs engine.EventSources

			opts, err := parseTrackerInputOptions(c.StringSlice("input-tracker"))
			if err == errHelp {
				printHelp()
				return nil
			}
			if err != nil {
				return err
			}

			inputs.Tracker, err = setupTrackerInputSource(opts)
			if err != nil {
				return err
			}

			output, err := setupOutput(
				os.Stdout,
				c.String("webhook"),
				c.String("webhook-template"),
				c.String("webhook-content-type"),
				c.String("output-template"),
			)
			if err != nil {
				return err
			}

			config := engine.Config{
				SignatureBufferSize: c.Uint(signatureBufferFlag),
				Signatures:          sigs,
				DataSources:         []detect.DataSource{},
			}
			e, err := engine.NewEngine(config, inputs, output)
			if err != nil {
				return fmt.Errorf("constructing engine: %w", err)
			}

			httpServer, err := server.PrepareServer(
				c.String(server.ListenEndpointFlag),
				c.Bool(server.MetricsEndpointFlag),
				c.Bool(server.HealthzEndpointFlag),
				c.Bool(server.PProfEndpointFlag),
				c.Bool(server.PyroscopeAgentFlag),
			)
			if err != nil {
				return err
			}

			err = e.Init()
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			if httpServer != nil {
				go httpServer.Start(ctx)
			}

			e.Start(ctx)

			return nil
		},
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "rules",
				Usage: "select which rules to load. Specify multiple rules by repeating this flag. Use --list for rules to select from",
			},
			&cli.StringFlag{
				Name:  "rules-dir",
				Usage: "directory where to search for rules in CEL (.yaml), OPA (.rego), and Go plugin (.so) formats",
			},
			&cli.BoolFlag{
				Name:  "rego-partial-eval",
				Usage: "enable partial evaluation of rego rules",
			},
			&cli.BoolFlag{
				Name:  "list",
				Usage: "print all available rules",
			},
			&cli.StringFlag{
				Name:  "webhook",
				Usage: "HTTP endpoint to call for every match",
			},
			&cli.StringFlag{
				Name:  "webhook-template",
				Usage: "path to a gotemplate for formatting webhook output",
			},
			&cli.StringFlag{
				Name:  "webhook-content-type",
				Usage: "content type of the template in use. Recommended if using --webhook-template",
			},
			&cli.StringSliceFlag{
				Name:  "input-tracker",
				Usage: "configure tracker-ebpf as input source. see '--input-tracker help' for more info",
			},
			&cli.StringFlag{
				Name:  "output-template",
				Usage: "configure output format via templates. Usage: --output-template=path/to/my.tmpl",
			},
			&cli.BoolFlag{
				Name:  server.PProfEndpointFlag,
				Usage: "enable pprof endpoints",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  server.PyroscopeAgentFlag,
				Usage: "enable pyroscope agent",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "rego-aio",
				Usage: "compile rego signatures altogether as an aggregate policy. By default each signature is compiled separately.",
			},
			&cli.StringFlag{
				Name:  "rego-runtime-target",
				Usage: "select which runtime target to use for evaluation of rego rules: rego, wasm",
				Value: "rego",
			},
			&cli.BoolFlag{
				Name:  "list-events",
				Usage: "print a list of events that currently loaded signatures require",
			},
			&cli.UintFlag{
				Name:  signatureBufferFlag,
				Usage: "size of the event channel's buffer consumed by signatures",
				Value: 1000,
			},
			&cli.BoolFlag{
				Name:  server.MetricsEndpointFlag,
				Usage: "enable metrics endpoint",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  server.HealthzEndpointFlag,
				Usage: "enable healthz endpoint",
				Value: false,
			},
			&cli.StringFlag{
				Name:  server.ListenEndpointFlag,
				Usage: "listening address of the metrics endpoint server",
				Value: ":4466",
			},
			&cli.BoolFlag{
				Name:  "allcaps",
				Value: false,
				Usage: "allow tracker-rules to run with all capabilities (use with caution)",
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		logger.Fatalw("App", "error", err)
	}
}

func listSigs(w io.Writer, sigs []detect.Signature) {
	fmt.Fprintf(w, "%-10s %-35s %s %s\n", "ID", "NAME", "VERSION", "DESCRIPTION")
	for _, sig := range sigs {
		meta, err := sig.GetMetadata()
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "%-10s %-35s %-7s %s\n", meta.ID, meta.Name, meta.Version, meta.Description)
	}
}

func listEvents(w io.Writer, sigs []detect.Signature) {
	m := make(map[string]struct{})
	for _, sig := range sigs {
		es, _ := sig.GetSelectedEvents()
		for _, e := range es {
			if _, ok := m[e.Name]; !ok {
				m[e.Name] = struct{}{}
			}
		}
	}

	var events []string
	for k := range m {
		events = append(events, k)
	}

	sort.Slice(events, func(i, j int) bool { return events[i] < events[j] })
	fmt.Fprintln(w, strings.Join(events, ","))
}

func isRoot() bool {
	return os.Geteuid() == 0
}
