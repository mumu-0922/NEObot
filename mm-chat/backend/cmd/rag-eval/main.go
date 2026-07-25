package main

import (
	"bytes"
	"crypto/sha256"
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
	promotionGoldenPath := flags.String(
		"promotion-golden",
		"",
		"path to a frozen human-reviewed promotion Golden corpus",
	)
	candidateObservationsPath := flags.String(
		"candidate-observations",
		"",
		"path to Candidate generation promotion observations",
	)
	candidateGenerationID := flags.String(
		"candidate-generation-id",
		"",
		"verified Candidate generation UUID",
	)
	artifactManifestHash := flags.String(
		"artifact-manifest-hash",
		"",
		"verified Candidate artifact manifest SHA-256",
	)
	printPromotionFreezeHash := flags.Bool(
		"print-promotion-freeze-hash",
		false,
		"print the canonical corpus freeze hash without admitting promotion",
	)
	pretty := flags.Bool("pretty", false, "indent the JSON report")
	if err := flags.Parse(args); err != nil {
		return errors.New("invalid arguments")
	}
	if flags.NArg() != 0 {
		return errors.New("invalid arguments")
	}
	legacyMode := *goldenPath != "" || *observationsPath != ""
	if *printPromotionFreezeHash {
		if legacyMode ||
			*promotionGoldenPath == "" ||
			*candidateObservationsPath != "" ||
			*candidateGenerationID != "" ||
			*artifactManifestHash != "" {
			return errors.New(
				"freeze-hash mode accepts only -promotion-golden",
			)
		}
		return runPromotionFreezeHash(*promotionGoldenPath, *pretty, output)
	}
	promotionMode := *promotionGoldenPath != "" ||
		*candidateObservationsPath != "" ||
		*candidateGenerationID != "" ||
		*artifactManifestHash != ""
	if legacyMode == promotionMode {
		return errors.New("select exactly one evaluation mode")
	}
	if legacyMode {
		return runLegacyEvaluation(
			*goldenPath,
			*observationsPath,
			*pretty,
			output,
		)
	}
	return runPromotionEvaluation(
		*promotionGoldenPath,
		*candidateObservationsPath,
		*candidateGenerationID,
		*artifactManifestHash,
		*pretty,
		output,
	)
}

func runPromotionFreezeHash(path string, pretty bool, output io.Writer) error {
	body, _, err := readHashedEvaluationFile(path)
	if err != nil {
		return fmt.Errorf("read promotion Golden corpus: %w", err)
	}
	golden, err := rageval.DecodePromotionGoldenSet(bytes.NewReader(body))
	if err != nil {
		return err
	}
	digest, err := rageval.PromotionGoldenContentSHA256(golden)
	if err != nil {
		return err
	}
	return encodeReport(output, struct {
		SchemaVersion       string `json:"schemaVersion"`
		CorpusID            string `json:"corpusId"`
		State               string `json:"state"`
		CaseCount           int    `json:"caseCount"`
		FrozenContentSHA256 string `json:"frozenContentSha256"`
		PromotionEligible   bool   `json:"promotionEligible"`
	}{
		SchemaVersion:       "neo-chat.rag-promotion-freeze-hash.v1",
		CorpusID:            golden.ID,
		State:               golden.Lifecycle.State,
		CaseCount:           len(golden.Cases),
		FrozenContentSHA256: digest,
		PromotionEligible:   false,
	}, pretty)
}

func runLegacyEvaluation(
	goldenPath string,
	observationsPath string,
	pretty bool,
	output io.Writer,
) error {
	if goldenPath == "" || observationsPath == "" {
		return errors.New("both -golden and -observations are required")
	}

	goldenFile, err := os.Open(goldenPath)
	if err != nil {
		return fmt.Errorf("open golden set: %w", err)
	}
	defer goldenFile.Close()
	golden, err := rageval.DecodeGoldenSet(goldenFile)
	if err != nil {
		return err
	}

	observationsFile, err := os.Open(observationsPath)
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
	if err := encodeReport(output, report, pretty); err != nil {
		return err
	}
	if !report.Passed {
		return errors.New("evaluation gate failed")
	}
	return nil
}

func runPromotionEvaluation(
	goldenPath string,
	candidateObservationsPath string,
	candidateGenerationID string,
	artifactManifestHash string,
	pretty bool,
	output io.Writer,
) error {
	if goldenPath == "" ||
		candidateObservationsPath == "" ||
		candidateGenerationID == "" ||
		artifactManifestHash == "" {
		return errors.New("all promotion evaluation inputs are required")
	}
	goldenBody, goldenHash, err := readHashedEvaluationFile(goldenPath)
	if err != nil {
		return fmt.Errorf("read promotion Golden corpus: %w", err)
	}
	golden, err := rageval.DecodePromotionGoldenSet(bytes.NewReader(goldenBody))
	if err != nil {
		return err
	}
	candidateBody, candidateHash, err := readHashedEvaluationFile(
		candidateObservationsPath,
	)
	if err != nil {
		return fmt.Errorf("read Candidate observations: %w", err)
	}
	candidate, err := rageval.DecodePromotionObservationSet(
		bytes.NewReader(candidateBody),
	)
	if err != nil {
		return err
	}
	report, err := rageval.EvaluatePromotion(rageval.PromotionEvaluationInput{
		Golden:                        golden,
		GoldenRawSHA256:               goldenHash,
		Candidate:                     candidate,
		CandidateRawSHA256:            candidateHash,
		CandidateGenerationID:         candidateGenerationID,
		CandidateArtifactManifestHash: artifactManifestHash,
	})
	if err != nil {
		return err
	}
	if err := encodeReport(output, report, pretty); err != nil {
		return err
	}
	if !report.Passed {
		return errors.New("promotion evaluation gate failed")
	}
	return nil
}

func readHashedEvaluationFile(path string) ([]byte, string, error) {
	const maximumEvaluationFileBytes = 64 << 20
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximumEvaluationFileBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maximumEvaluationFileBytes {
		return nil, "", errors.New("evaluation file exceeds 64 MiB")
	}
	digest := sha256.Sum256(body)
	return body, fmt.Sprintf("%x", digest), nil
}

func encodeReport(output io.Writer, report any, pretty bool) error {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(report); err != nil {
		return errors.New("write report")
	}
	return nil
}
