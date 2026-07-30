package memorycapture

import (
	"errors"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
)

func TestBuildMemoryToolRouteDiagnosticReportRetainsFailClosedRetrievalFailure(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingMemoryToolRouteDevelopmentTraces(pool)
	trace := &traces[0]
	trace.AdmissionReady = false
	trace.RerankReady = false
	trace.AbstentionCode = "RELEVANCE_ADMISSION_UNAVAILABLE"
	trace.FullObservation.FinalMemoryIDs = []string{}
	trace.FullObservation.InjectedMemoryIDs = []string{}
	trace.FullObservation.ProviderSentMemoryIDs = []string{}
	trace.FullObservation.PromptMemoryTokens = 0
	trace.FullObservation.Fallback = "no_memory"
	profile := memoryToolRouteDevelopmentProfile(traces)
	_, _, legacyErr := BuildMemoryToolRouteDevelopmentReport(
		pool,
		profile,
		memoryToolRouteTestAuthority(),
		memoryToolRouteTestCostBasis(),
	)
	if legacyErr == nil || !errors.Is(legacyErr, ErrCaptureInvalid) ||
		legacyErr.Error() != "native Memory capture is invalid: Memory Tool-route report admission_state" {
		t.Fatalf("schema-v7 admission error = %v", legacyErr)
	}
	profile.Profile.ReaderVersion = MemoryToolFirstRoundDiagnosticReaderVersion
	report, body, err := BuildMemoryToolRouteDiagnosticReport(
		pool,
		profile,
		memoryToolRouteTestAuthority(),
		memoryToolRouteTestCostBasis(),
	)
	if err != nil || report.Diagnostics.RetrievalIncompleteCount != 1 ||
		report.Diagnostics.RetrievalFailureCodeCounts["RELEVANCE_ADMISSION_UNAVAILABLE"] != 1 ||
		report.Diagnostics.RouteCompletedCaseCount != 300 ||
		report.Diagnostics.FailedCaseCount != 0 ||
		strings.Contains(string(body), trace.CaseID) {
		t.Fatalf("fail-closed retrieval diagnostic=%#v err=%v body=%s", report, err, body)
	}
}

func TestBuildMemoryToolRouteDiagnosticReportRetainsFailClosedRerankFailure(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingMemoryToolRouteDevelopmentTraces(pool)
	trace := &traces[0]
	trace.RerankReady = false
	trace.AbstentionCode = "RERANK_FAILED"
	trace.FullObservation.FinalMemoryIDs = []string{}
	trace.FullObservation.InjectedMemoryIDs = []string{}
	trace.FullObservation.PromptMemoryTokens = 0
	trace.FullObservation.Fallback = "no_memory"
	profile := memoryToolRouteDevelopmentProfile(traces)
	profile.Profile.ReaderVersion = MemoryToolFirstRoundDiagnosticReaderVersion
	report, body, err := BuildMemoryToolRouteDiagnosticReport(
		pool,
		profile,
		memoryToolRouteTestAuthority(),
		memoryToolRouteTestCostBasis(),
	)
	if err != nil || report.Diagnostics.RetrievalIncompleteCount != 1 ||
		report.Diagnostics.RetrievalFailureCodeCounts["RERANK_FAILED"] != 1 ||
		report.Diagnostics.RouteCompletedCaseCount != 300 ||
		report.Diagnostics.FailedCaseCount != 0 ||
		strings.Contains(string(body), trace.CaseID) {
		t.Fatalf("fail-closed rerank diagnostic=%#v err=%v body=%s", report, err, body)
	}
}

func TestBuildMemoryToolRouteDiagnosticReportRejectsIncompleteRetrievalWithFinal(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingMemoryToolRouteDevelopmentTraces(pool)
	trace := &traces[0]
	trace.AdmissionReady = false
	trace.RerankReady = false
	profile := memoryToolRouteDevelopmentProfile(traces)
	profile.Profile.ReaderVersion = MemoryToolFirstRoundDiagnosticReaderVersion
	_, _, err = BuildMemoryToolRouteDiagnosticReport(
		pool,
		profile,
		memoryToolRouteTestAuthority(),
		memoryToolRouteTestCostBasis(),
	)
	if err == nil || !errors.Is(err, ErrCaptureInvalid) ||
		err.Error() != "native Memory capture is invalid: Memory Tool-route report incomplete_retrieval_state" ||
		strings.Contains(err.Error(), trace.CaseID) {
		t.Fatalf("bounded integrity error = %v", err)
	}
}
