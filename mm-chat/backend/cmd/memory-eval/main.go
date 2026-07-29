package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "memory-eval:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("memory-eval", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	goldenPath := flags.String("golden", "", "path to a Memory Golden corpus")
	regressionCorpusPath := flags.String("regression-corpus", "", "path to a machine-reviewed regression corpus")
	regressionAuditPath := flags.String("regression-audit", "", "path to the bound regression audit")
	observationsPath := flags.String("observations", "", "path to Memory benchmark observations")
	outputPath := flags.String("output", "", "new exclusive report JSON path")
	printFreezeHash := flags.Bool(
		"print-freeze-hash",
		false,
		"print the canonical corpus hash without admitting promotion",
	)
	pretty := flags.Bool("pretty", false, "indent JSON output")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("invalid arguments")
	}
	*goldenPath = strings.TrimSpace(*goldenPath)
	*regressionCorpusPath = strings.TrimSpace(*regressionCorpusPath)
	*regressionAuditPath = strings.TrimSpace(*regressionAuditPath)
	*observationsPath = strings.TrimSpace(*observationsPath)
	*outputPath = strings.TrimSpace(*outputPath)
	regressionMode := *regressionCorpusPath != "" || *regressionAuditPath != ""
	if *printFreezeHash {
		if *goldenPath == "" || regressionMode || *observationsPath != "" || *outputPath != "" {
			return errors.New("freeze-hash mode accepts only -golden")
		}
		return runFreezeHash(*goldenPath, *pretty, stdout)
	}
	if regressionMode {
		if *goldenPath != "" || *regressionCorpusPath == "" || *regressionAuditPath == "" ||
			*observationsPath == "" || *outputPath == "" {
			return errors.New("regression mode requires -regression-corpus, -regression-audit, -observations, and -output only")
		}
		return runRegressionEvaluation(
			*regressionCorpusPath,
			*regressionAuditPath,
			*observationsPath,
			*outputPath,
			*pretty,
			stdout,
		)
	}
	if *goldenPath == "" || *observationsPath == "" || *outputPath == "" {
		return errors.New("-golden, -observations, and -output are required")
	}
	return runEvaluation(
		*goldenPath,
		*observationsPath,
		*outputPath,
		*pretty,
		stdout,
	)
}

func runRegressionEvaluation(
	corpusPath string,
	auditPath string,
	observationsPath string,
	outputPath string,
	pretty bool,
	stdout io.Writer,
) error {
	corpusBody, corpusRawHash, err := readHashedFile(corpusPath)
	if err != nil {
		return fmt.Errorf("read Memory regression corpus: %w", err)
	}
	corpus, err := memoryeval.DecodeRegressionCorpus(bytes.NewReader(corpusBody))
	if err != nil {
		return err
	}
	auditBody, auditRawHash, err := readHashedFile(auditPath)
	if err != nil {
		return fmt.Errorf("read Memory regression audit: %w", err)
	}
	audit, err := memoryeval.DecodeRegressionAudit(bytes.NewReader(auditBody))
	if err != nil {
		return err
	}
	observationBody, observationRawHash, err := readHashedFile(observationsPath)
	if err != nil {
		return fmt.Errorf("read Memory regression observations: %w", err)
	}
	observations, err := memoryeval.DecodeRegressionObservationSet(bytes.NewReader(observationBody))
	if err != nil {
		return err
	}
	report, err := memoryeval.EvaluateRegression(memoryeval.RegressionEvaluationInput{
		Corpus:             corpus,
		CorpusRawSHA256:    corpusRawHash,
		Audit:              audit,
		AuditRawSHA256:     auditRawHash,
		Observations:       observations,
		ObservationsSHA256: observationRawHash,
	})
	if err != nil {
		return err
	}
	reportHash, err := writeReportExclusive(outputPath, report, pretty)
	if err != nil {
		return err
	}
	if err := encodeJSON(stdout, struct {
		SchemaVersion     string `json:"schemaVersion"`
		OutputPath        string `json:"outputPath"`
		ReportSHA256      string `json:"reportSha256"`
		Passed            bool   `json:"passed"`
		PromotionEligible bool   `json:"promotionEligible"`
	}{
		SchemaVersion:     "neo-chat.memory-benchmark-regression-report-output.v1",
		OutputPath:        outputPath,
		ReportSHA256:      reportHash,
		Passed:            report.Passed,
		PromotionEligible: false,
	}, pretty); err != nil {
		return err
	}
	if !report.Passed {
		return errors.New("Memory regression benchmark gate failed")
	}
	return nil
}

func runFreezeHash(path string, pretty bool, stdout io.Writer) error {
	body, _, err := readHashedFile(path)
	if err != nil {
		return fmt.Errorf("read Memory Golden corpus: %w", err)
	}
	golden, err := memoryeval.DecodeGoldenSet(bytes.NewReader(body))
	if err != nil {
		return err
	}
	report, err := memoryeval.NewFreezeHashReport(golden)
	if err != nil {
		return err
	}
	return encodeJSON(stdout, report, pretty)
}

func runEvaluation(
	goldenPath string,
	observationsPath string,
	outputPath string,
	pretty bool,
	stdout io.Writer,
) error {
	goldenBody, goldenRawHash, err := readHashedFile(goldenPath)
	if err != nil {
		return fmt.Errorf("read Memory Golden corpus: %w", err)
	}
	golden, err := memoryeval.DecodeGoldenSet(bytes.NewReader(goldenBody))
	if err != nil {
		return err
	}
	observationBody, observationRawHash, err := readHashedFile(observationsPath)
	if err != nil {
		return fmt.Errorf("read Memory observations: %w", err)
	}
	observations, err := memoryeval.DecodeObservationSet(bytes.NewReader(observationBody))
	if err != nil {
		return err
	}
	report, err := memoryeval.Evaluate(memoryeval.EvaluationInput{
		Golden:             golden,
		GoldenRawSHA256:    goldenRawHash,
		Observations:       observations,
		ObservationsSHA256: observationRawHash,
	})
	if err != nil {
		return err
	}
	reportHash, err := writeReportExclusive(outputPath, report, pretty)
	if err != nil {
		return err
	}
	if err := encodeJSON(stdout, struct {
		SchemaVersion string `json:"schemaVersion"`
		OutputPath    string `json:"outputPath"`
		ReportSHA256  string `json:"reportSha256"`
		Passed        bool   `json:"passed"`
	}{
		SchemaVersion: "neo-chat.memory-benchmark-report-output.v1",
		OutputPath:    outputPath,
		ReportSHA256:  reportHash,
		Passed:        report.Passed,
	}, pretty); err != nil {
		return err
	}
	if !report.Passed {
		return errors.New("Memory benchmark gate failed")
	}
	return nil
}

func readHashedFile(path string) ([]byte, string, error) {
	const maximumFileBytes = 64 << 20
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximumFileBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) == 0 || len(body) > maximumFileBytes {
		return nil, "", errors.New("evaluation file size is invalid")
	}
	digest := sha256.Sum256(body)
	return body, hex.EncodeToString(digest[:]), nil
}

func writeReportExclusive(
	path string,
	report any,
	pretty bool,
) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("Memory benchmark report output is invalid")
	}
	body, err := marshalJSON(report, pretty)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", errors.New("create Memory benchmark report directory")
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".memory-eval-*.tmp")
	if err != nil {
		return "", errors.New("create Memory benchmark report temporary file")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", errors.New("secure Memory benchmark report temporary file")
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return "", errors.New("write Memory benchmark report")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", errors.New("sync Memory benchmark report")
	}
	if err := temporary.Close(); err != nil {
		return "", errors.New("close Memory benchmark report")
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", errors.New("Memory benchmark report output already exists")
		}
		return "", errors.New("publish Memory benchmark report")
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func encodeJSON(output io.Writer, value any, pretty bool) error {
	body, err := marshalJSON(value, pretty)
	if err != nil {
		return err
	}
	if _, err := output.Write(body); err != nil {
		return errors.New("write JSON output")
	}
	return nil
}

func marshalJSON(value any, pretty bool) ([]byte, error) {
	var body []byte
	var err error
	if pretty {
		body, err = json.MarshalIndent(value, "", "  ")
	} else {
		body, err = json.Marshal(value)
	}
	if err != nil {
		return nil, errors.New("encode JSON output")
	}
	return append(body, '\n'), nil
}
