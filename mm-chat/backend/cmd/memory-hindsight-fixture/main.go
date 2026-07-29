package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/hindsightfixture"
	"neo-chat/mm-chat/backend/internal/memoryeval"
)

const hindsightFixtureAPIURL = "http://hindsight-api:8888"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "memory-hindsight-fixture: fixture comparison failed")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("memory-hindsight-fixture", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "path to the synthetic fixture manifest")
	goldenPath := flags.String("golden", "", "path to the bound Memory Golden corpus")
	modeValue := flags.String("mode", "", "end_to_end or retrieval_only")
	printManifestHash := flags.Bool(
		"print-manifest-hash",
		false,
		"print the canonical synthetic manifest hash without making a request",
	)
	pretty := flags.Bool("pretty", false, "indent the content-free report")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("invalid arguments")
	}
	*manifestPath = strings.TrimSpace(*manifestPath)
	*goldenPath = strings.TrimSpace(*goldenPath)
	mode := hindsightfixture.Mode(strings.TrimSpace(*modeValue))
	if *printManifestHash {
		if *manifestPath == "" || *goldenPath != "" || *modeValue != "" {
			return errors.New("manifest-hash mode accepts only -manifest")
		}
		return runManifestHash(*manifestPath, *pretty, stdout)
	}
	if *manifestPath == "" || *goldenPath == "" ||
		(mode != hindsightfixture.ModeEndToEnd && mode != hindsightfixture.ModeRetrievalOnly) {
		return errors.New("manifest, Golden corpus, and fixed mode are required")
	}
	manifestFile, err := os.Open(*manifestPath)
	if err != nil {
		return errors.New("open fixture manifest")
	}
	manifest, err := hindsightfixture.DecodeManifest(manifestFile)
	_ = manifestFile.Close()
	if err != nil {
		return errors.New("validate fixture manifest")
	}
	goldenBody, goldenHash, err := readGolden(*goldenPath)
	if err != nil {
		return err
	}
	golden, err := memoryeval.DecodeGoldenSet(bytes.NewReader(goldenBody))
	if err != nil {
		return errors.New("validate Memory Golden corpus")
	}
	apiKey := os.Getenv("HINDSIGHT_FIXTURE_API_KEY")
	client, err := hindsightfixture.NewHTTPClient(
		hindsightFixtureAPIURL,
		apiKey,
		&http.Client{Timeout: 120 * time.Second},
	)
	if err != nil {
		return errors.New("configure fixture adapter")
	}
	runner, err := hindsightfixture.NewRunner(client, apiKey)
	if err != nil {
		return errors.New("configure fixture runner")
	}
	report := runner.Run(ctx, manifest, golden, goldenHash, mode)
	if err := writeReport(stdout, report, *pretty); err != nil {
		return err
	}
	if !report.Passed {
		return errors.New("fixture report failed")
	}
	return nil
}

func runManifestHash(path string, pretty bool, stdout io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.New("open fixture manifest")
	}
	manifest, err := hindsightfixture.DecodeManifestForHash(file)
	_ = file.Close()
	if err != nil {
		return errors.New("validate fixture manifest")
	}
	digest, err := hindsightfixture.ManifestContentSHA256(manifest)
	if err != nil {
		return errors.New("hash fixture manifest")
	}
	return writeJSON(stdout, struct {
		SchemaVersion     string `json:"schemaVersion"`
		ManifestID        string `json:"manifestId"`
		ContentSHA256     string `json:"contentSha256"`
		PromotionEligible bool   `json:"promotionEligible"`
	}{
		SchemaVersion:     "neo-chat.memory-hindsight-fixture-hash.v1",
		ManifestID:        manifest.ID,
		ContentSHA256:     digest,
		PromotionEligible: false,
	}, pretty)
}

func readGolden(path string) ([]byte, string, error) {
	const maximumGoldenBytes = 64 << 20
	file, err := os.Open(path)
	if err != nil {
		return nil, "", errors.New("open Memory Golden corpus")
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximumGoldenBytes+1))
	if err != nil || len(body) == 0 || len(body) > maximumGoldenBytes {
		return nil, "", errors.New("read Memory Golden corpus")
	}
	digest := sha256.Sum256(body)
	return body, hex.EncodeToString(digest[:]), nil
}

func writeReport(output io.Writer, report hindsightfixture.Report, pretty bool) error {
	return writeJSON(output, report, pretty)
}

func writeJSON(output io.Writer, value any, pretty bool) error {
	var body []byte
	var err error
	if pretty {
		body, err = json.MarshalIndent(value, "", "  ")
	} else {
		body, err = json.Marshal(value)
	}
	if err != nil {
		return errors.New("encode fixture report")
	}
	body = append(body, '\n')
	if _, err := output.Write(body); err != nil {
		return errors.New("write fixture report")
	}
	return nil
}
