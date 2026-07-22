package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"neo-chat/mm-chat/backend/internal/rageval"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "rag-eval:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("rag-eval", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	goldenPath := flags.String("golden", "", "path to a versioned Golden Set JSON file")
	observationsPath := flags.String("observations", "", "path to a redacted observation JSON file")
	pretty := flags.Bool("pretty", false, "indent the JSON report")
	if err := flags.Parse(args); err != nil {
		return errors.New("invalid arguments")
	}
	if flags.NArg() != 0 || *goldenPath == "" || *observationsPath == "" {
		return errors.New("both -golden and -observations are required")
	}

	goldenFile, err := os.Open(*goldenPath)
	if err != nil {
		return fmt.Errorf("open golden set: %w", err)
	}
	defer goldenFile.Close()
	golden, err := rageval.DecodeGoldenSet(goldenFile)
	if err != nil {
		return err
	}

	observationsFile, err := os.Open(*observationsPath)
	if err != nil {
		return fmt.Errorf("open observations: %w", err)
	}
	defer observationsFile.Close()
	observations, err := rageval.DecodeObservationSet(observationsFile)
	if err != nil {
		return err
	}

	report, err := rageval.Evaluate(golden, observations)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	if *pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(report); err != nil {
		return errors.New("write report")
	}
	if !report.Passed {
		return errors.New("evaluation gate failed")
	}
	return nil
}
