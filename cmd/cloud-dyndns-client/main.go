// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/ianlewis/cloud-dyndns-client/pkg/backend"
	"github.com/ianlewis/cloud-dyndns-client/pkg/backend/gcp"
	"github.com/ianlewis/cloud-dyndns-client/pkg/sync"
)

// VERSION is the current version of the application.
var VERSION = "0.1.5"

// Domain is a single domain listed in the configuration file.
type Domain struct {
	Provider       string                 `json:"provider"`
	ProviderConfig map[string]interface{} `json:"provider_config"`
	Backend        backend.DNSBackend
}

// Config is the configuration contained in the given configuration file.
type Config struct {
	Domains map[string]*Domain `json:"domains"`
}

func getFileContents(pathToFile string) ([]byte, error) {
	// Open file
	f, err := os.Open(pathToFile)
	if err != nil {
		return []byte{}, fmt.Errorf("Could not open %s: %v", pathToFile, err)
	}
	defer f.Close()

	contents, err := ioutil.ReadAll(f)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to read	%s: %v", f.Name(), err)
	}

	return contents, nil
}

func getConfig(pathToJSON string) (Config, error) {
	var cfg Config

	jsonContents, err := getFileContents(pathToJSON)
	if err != nil {
		return cfg, err
	}

	err = json.Unmarshal(jsonContents, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("Could not load %s: %v", pathToJSON, err)
	}

	for _, d := range cfg.Domains {
		if d.Provider == "gcp" {
			p, ok := d.ProviderConfig["project_id"]
			if !ok {
				return cfg, fmt.Errorf("\"project_id\" is required for Cloud DNS config")
			}
			project, ok := p.(string)
			if !ok {
				return cfg, fmt.Errorf("\"project_id\" must be a string")
			}

			z, ok := d.ProviderConfig["managed_zone"]
			if !ok {
				return cfg, fmt.Errorf("\"managed_zone\" is required for Cloud DNS config")
			}
			zone, ok := z.(string)
			if !ok {
				return cfg, fmt.Errorf("\"managed_zone\" must be a string")
			}

			b, err := gcp.NewCloudDNSBackend(project, zone)
			if err != nil {
				return cfg, fmt.Errorf("Could not create Cloud DNS backend: %v", err)
			}
			d.Backend = b
		} else {
			return cfg, fmt.Errorf("Unknown backend provider: %s", d.Provider)
		}
	}

	return cfg, nil
}

// Main is the main function for the cloud-dyndns-client command. It returns the OS exit code.
func main() {
	addr := flag.String("addr", ":8080", "Address to listen on for health checks.")
	version := flag.Bool("version", false, "Print the version and exit.")
	config := flag.String("config", "/etc/cloud-dyndns-client/config.json", "The path to the JSON config file.")

	flag.Parse()

	if *version {
		fmt.Println(VERSION)
		return
	}

	cfg, err := getConfig(*config)
	if err != nil {
		log.Fatalf("Error reading config: %v", err)
	}

	// Convert config to sync records
	records := []sync.Record{}
	for name, d := range cfg.Domains {
		if !strings.HasSuffix(name, ".") {
			name = name + "."
		}
		records = append(records, sync.Record{
			Record: backend.NewDNSRecord(
				name,
				"A",
				600,
				[]string{},
			),
			Backend: d.Backend,
		})
	}

	// Create a new syncer. This will sync DNS records to backends
	// and ensure records are set to the desired values.
	syncer := sync.NewSyncer(records, 30*time.Second, 5*time.Second)

	// The IP Address poller will poll for the Internet IP address.
	// When a new address is polled the data will be forwarded to the syncer.
	poller := sync.NewIPAddressPoller(5 * time.Minute)

	// Create a waitgroup to manage the goroutines for the main loops.
	// The waitgroup can be used to wait for goroutines to finish.
	ctx, cancel := context.WithCancel(context.Background())
	wg, ctx := errgroup.WithContext(ctx)

	// TODO: Refactor and move this code to it's own package
	wg.Go(func() error { return syncer.Run(ctx.Done()) })
	wg.Go(func() error { return poller.Run(ctx.Done()) })
	wg.Go(func() error {
		// This goroutine receives IP address polling results
		// and updates the desired records in the Syncer.
		c := poller.Channel()
		for {
			select {
			case ip := <-c:
				for _, r := range records {
					syncer.UpdateRecord(
						r.Record.Name(),
						r.Record.Type(),
						r.Record.Ttl(),
						[]string{ip},
					)
				}
			case <-ctx.Done():
				return nil
			}
		}
	})
	// TODO: Refactor and move to it's own package
	wg.Go(func() error {
		// This goroutine sets up health checks on an HTTP endpoint.
		// It's a bit complicated as it is necessary to gracefully
		// shutdown the http server.
		mux := http.NewServeMux()
		mux.HandleFunc("/_status/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		})

		srv := &http.Server{
			Addr:    *addr,
			Handler: mux,
		}

		// Since srv.ListenAndServe() blocks we need to start it in
		// a goroutine so the select can monitor the context's done
		// channel as well as if the server returns an error.
		errChan := make(chan error, 1)
		go func(errChan chan error) {
			log.Printf("Listening on %s...", *addr)
			errChan <- srv.ListenAndServe()
		}(errChan)

		select {
		case <-ctx.Done():
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			return srv.Shutdown(shutdownCtx)
		case err := <-errChan:
			return err
		}
	})

	// Wait for SIGINT or SIGTERM signals and shutdown the application if
	// one is received.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case s := <-signals:
		log.Printf("Received signal %v, exiting...", s)
	case <-ctx.Done():
	}
	cancel()

	if err := wg.Wait(); err != nil {
		log.Fatalf("Unhandled error received. Exiting: %v", err)
		os.Exit(1)
	}
}
