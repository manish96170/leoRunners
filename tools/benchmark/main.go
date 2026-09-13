package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	mode := flag.String("mode", "fake", "benchmark mode: fake or real-aws")
	format := flag.String("format", "json", "output format: json or csv")
	output := flag.String("output", "-", "output file, or - for stdout")
	count := flag.Int("count", 1, "number of deterministic fake timelines to replay")
	realAWS := flag.Bool("allow-real-aws", false, "explicitly acknowledge that real-aws mode may make cloud calls")
	flag.Parse()

	if *mode == "real-aws" {
		if !*realAWS {
			fail(errors.New("real-aws mode is disabled by default; repeat with --allow-real-aws after reviewing docs/real-aws.md"))
		}
		fail(errors.New("real-aws collection is an opt-in integration hook; feed lifecycle marks into Collector from your AWS runner job"))
	}
	if *mode != "fake" {
		fail(fmt.Errorf("unsupported mode %q", *mode))
	}
	if *count < 1 {
		fail(errors.New("count must be positive"))
	}
	if *format != "json" && *format != "csv" {
		fail(fmt.Errorf("unsupported format %q", *format))
	}

	samples := make([]Sample, 0, *count)
	for i := 0; i < *count; i++ {
		sample, err := FakeTimeline{ID: fmt.Sprintf("fake-%04d", i+1), Start: time.Unix(int64(i)*3600, 0).UTC(), Durations: defaultDurations()}.Replay()
		if err != nil {
			fail(err)
		}
		samples = append(samples, sample)
	}
	var file *os.File
	writer := os.Stdout
	if *output != "-" {
		var err error
		file, err = os.Create(*output)
		if err != nil {
			fail(err)
		}
		defer file.Close()
		writer = file
	}
	var err error
	if *format == "csv" {
		err = WriteCSV(writer, samples)
	} else {
		err = WriteJSON(writer, samples)
	}
	if err != nil {
		fail(err)
	}
}

func defaultDurations() map[Phase]time.Duration {
	return map[Phase]time.Duration{Queue: 250 * time.Millisecond, Launch: 4 * time.Second, Boot: 12 * time.Second, Registration: 3 * time.Second, Ready: 8 * time.Second, JobStart: 2 * time.Second, Cleanup: 1 * time.Second}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "benchmark:", err); os.Exit(1) }
