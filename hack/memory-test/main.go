// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// memory-test profiles memory usage during UDS bundle operations.
//
// Usage:
//
//	go run ./hack/memory-test create --source ./bundle-dir
//	go run ./hack/memory-test publish --bundle ./bundle.tar.zst --registry oci://localhost:5000
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/src/types"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "create":
		runCreate(os.Args[2:])
	case "publish":
		runPublish(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`UDS Memory Profiler

Usage:
  memory-test <command> [flags]

Commands:
  create    Profile memory during bundle creation
  publish   Profile memory during bundle publish

Examples:
  go run ./hack/memory-test create --source ./my-bundle
  go run ./hack/memory-test publish --bundle ./bundle.tar.zst --registry oci://localhost:5000
  go run ./hack/memory-test create --source ./my-bundle --max-memory 512
`)
}

// Profiler tracks memory usage during an operation
type Profiler struct {
	interval    time.Duration
	outputDir   string
	maxMemoryMB float64
	writeCSV    bool

	samples    []Sample
	baseline   Sample
	peak       Sample
	stopCh     chan struct{}
	done       chan struct{}
}

// Sample holds memory stats at a point in time
type Sample struct {
	Time       time.Time
	HeapAlloc  uint64
	HeapSys    uint64
	Sys        uint64
	NumGC      uint32
	TotalAlloc uint64
}

func newProfiler(interval time.Duration, outputDir string, maxMemoryMB float64, writeCSV bool) *Profiler {
	return &Profiler{
		interval:    interval,
		outputDir:   outputDir,
		maxMemoryMB: maxMemoryMB,
		writeCSV:    writeCSV,
		samples:     make([]Sample, 0, 1000),
		stopCh:      make(chan struct{}),
		done:        make(chan struct{}),
	}
}

func captureSample() Sample {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return Sample{
		Time:       time.Now(),
		HeapAlloc:  m.HeapAlloc,
		HeapSys:    m.HeapSys,
		Sys:        m.Sys,
		NumGC:      m.NumGC,
		TotalAlloc: m.TotalAlloc,
	}
}

func (p *Profiler) start() {
	runtime.GC()
	p.baseline = captureSample()
	p.peak = p.baseline

	go func() {
		defer close(p.done)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s := captureSample()
				p.samples = append(p.samples, s)
				if s.HeapAlloc > p.peak.HeapAlloc {
					p.peak = s
					fmt.Printf("  [PEAK] %s\n", formatBytes(s.HeapAlloc))
				}
			case <-p.stopCh:
				return
			}
		}
	}()
}

func (p *Profiler) stop() {
	close(p.stopCh)
	<-p.done
	p.samples = append(p.samples, captureSample())
}

func (p *Profiler) report(duration time.Duration) {
	final := p.samples[len(p.samples)-1]

	fmt.Printf(`
================== MEMORY REPORT ==================
Baseline:        %s
Peak:            %s
Final:           %s
Peak Δ:          %s
Total Allocated: %s
GC Cycles:       %d
Duration:        %s
Samples:         %d
===================================================
`,
		formatBytes(p.baseline.HeapAlloc),
		formatBytes(p.peak.HeapAlloc),
		formatBytes(final.HeapAlloc),
		formatBytes(p.peak.HeapAlloc-p.baseline.HeapAlloc),
		formatBytes(final.TotalAlloc-p.baseline.TotalAlloc),
		final.NumGC-p.baseline.NumGC,
		duration.Round(time.Millisecond),
		len(p.samples),
	)
}

func (p *Profiler) writeOutputs() error {
	if p.outputDir == "" {
		return nil
	}

	ts := time.Now().Format("20060102-150405")
	dir := filepath.Join(p.outputDir, ts)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write heap profile
	profPath := filepath.Join(dir, "heap.pprof")
	f, err := os.Create(profPath)
	if err != nil {
		return err
	}
	if err := pprof.WriteHeapProfile(f); err != nil {
		f.Close()
		return err
	}
	f.Close()
	fmt.Printf("Heap profile: %s\n", profPath)

	// Write CSV
	if p.writeCSV && len(p.samples) > 0 {
		csvPath := filepath.Join(dir, "samples.csv")
		cf, err := os.Create(csvPath)
		if err != nil {
			return err
		}
		fmt.Fprintln(cf, "elapsed_ms,heap_alloc,heap_sys,sys,total_alloc,num_gc")
		base := p.samples[0].Time
		for _, s := range p.samples {
			fmt.Fprintf(cf, "%d,%d,%d,%d,%d,%d\n",
				s.Time.Sub(base).Milliseconds(),
				s.HeapAlloc, s.HeapSys, s.Sys, s.TotalAlloc, s.NumGC)
		}
		cf.Close()
		fmt.Printf("CSV samples: %s\n", csvPath)
	}

	return nil
}

func (p *Profiler) checkLimit() bool {
	if p.maxMemoryMB <= 0 {
		return true
	}
	peakMB := float64(p.peak.HeapAlloc) / (1024 * 1024)
	if peakMB > p.maxMemoryMB {
		fmt.Fprintf(os.Stderr, "FAILED: Peak %.2f MB exceeded limit %.2f MB\n", peakMB, p.maxMemoryMB)
		return false
	}
	return true
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func runCreate(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	source := fs.String("source", ".", "Source directory with uds-bundle.yaml")
	output := fs.String("output", ".", "Output directory for bundle")
	interval := fs.Duration("interval", 100*time.Millisecond, "Sample interval")
	outputDir := fs.String("output-dir", "", "Directory for profile outputs")
	maxMem := fs.Float64("max-memory", 0, "Fail if peak exceeds MB (0=unlimited)")
	csv := fs.Bool("csv", true, "Write CSV samples")
	concurrency := fs.Int("concurrency", 3, "OCI concurrency")
	insecure := fs.Bool("insecure", true, "Allow insecure connections")

	fs.Parse(args)

	bundleFile := filepath.Join(*source, config.BundleYAML)
	if _, err := os.Stat(bundleFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s not found\n", bundleFile)
		os.Exit(1)
	}

	fmt.Printf("Creating bundle from %s\n", *source)

	config.CommonOptions.Confirm = true
	config.CommonOptions.Insecure = *insecure
	config.CommonOptions.OCIConcurrency = *concurrency

	cfg := &types.BundleConfig{
		CreateOpts: types.BundleCreateOptions{
			SourceDirectory: *source,
			Output:          *output,
			BundleFile:      config.BundleYAML,
		},
	}

	b, err := bundle.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer b.ClearPaths()

	prof := newProfiler(*interval, *outputDir, *maxMem, *csv)
	prof.start()
	start := time.Now()

	err = b.Create(context.Background())

	prof.stop()
	duration := time.Since(start)

	prof.report(duration)
	if *outputDir != "" {
		if werr := prof.writeOutputs(); werr != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", werr)
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Create failed: %v\n", err)
		os.Exit(1)
	}
	if !prof.checkLimit() {
		os.Exit(1)
	}
	fmt.Println("Create completed successfully")
}

func runPublish(args []string) {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	bundlePath := fs.String("bundle", "", "Bundle tarball path (required)")
	registry := fs.String("registry", "oci://localhost:5000", "OCI registry URL")
	interval := fs.Duration("interval", 100*time.Millisecond, "Sample interval")
	outputDir := fs.String("output-dir", "", "Directory for profile outputs")
	maxMem := fs.Float64("max-memory", 0, "Fail if peak exceeds MB (0=unlimited)")
	csv := fs.Bool("csv", true, "Write CSV samples")
	concurrency := fs.Int("concurrency", 3, "OCI concurrency")
	insecure := fs.Bool("insecure", true, "Allow insecure connections")

	fs.Parse(args)

	if *bundlePath == "" {
		fmt.Fprintln(os.Stderr, "Error: --bundle is required")
		fs.Usage()
		os.Exit(1)
	}

	info, err := os.Stat(*bundlePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Publishing %s (%s) to %s\n", *bundlePath, formatBytes(uint64(info.Size())), *registry)

	config.CommonOptions.Insecure = *insecure
	config.CommonOptions.OCIConcurrency = *concurrency

	cfg := &types.BundleConfig{
		PublishOpts: types.BundlePublishOptions{
			Source:      *bundlePath,
			Destination: *registry,
		},
	}

	b, err := bundle.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer b.ClearPaths()

	prof := newProfiler(*interval, *outputDir, *maxMem, *csv)
	prof.start()
	start := time.Now()

	err = b.Publish()

	prof.stop()
	duration := time.Since(start)

	prof.report(duration)
	if *outputDir != "" {
		if werr := prof.writeOutputs(); werr != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", werr)
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Publish failed: %v\n", err)
		os.Exit(1)
	}
	if !prof.checkLimit() {
		os.Exit(1)
	}
	fmt.Println("Publish completed successfully")
}
