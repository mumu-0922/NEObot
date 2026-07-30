package memorycapture

import "encoding/json"

// MarshalJSON keeps v7 bytes free of diagnostic-only fields while requiring
// v9 to publish explicit empty aggregate maps instead of silently omitting
// zero-failure evidence through encoding/json's map omitempty behavior.
func (value MemoryToolRouteDevelopmentDiagnostics) MarshalJSON() ([]byte, error) {
	type wireDiagnostics struct {
		EmptyCandidateCaseCount    int             `json:"emptyCandidateCaseCount"`
		RouteCompletedCaseCount    int             `json:"routeCompletedCaseCount"`
		RouteUsedCaseCount         int             `json:"routeUsedCaseCount"`
		RouteAbstainedCaseCount    int             `json:"routeAbstainedCaseCount"`
		FailedCaseCount            int             `json:"failedCaseCount"`
		FailureCodeCounts          map[string]int  `json:"failureCodeCounts"`
		RouteFailureCategoryCounts *map[string]int `json:"routeFailureCategoryCounts,omitempty"`
		RetrievalIncompleteCount   *int            `json:"retrievalIncompleteCaseCount,omitempty"`
		RetrievalFailureCodeCounts *map[string]int `json:"retrievalFailureCodeCounts,omitempty"`
	}
	wire := wireDiagnostics{
		EmptyCandidateCaseCount: value.EmptyCandidateCaseCount,
		RouteCompletedCaseCount: value.RouteCompletedCaseCount,
		RouteUsedCaseCount:      value.RouteUsedCaseCount,
		RouteAbstainedCaseCount: value.RouteAbstainedCaseCount,
		FailedCaseCount:         value.FailedCaseCount,
		FailureCodeCounts:       value.FailureCodeCounts,
	}
	if value.RouteFailureCategoryCounts != nil {
		if value.RetrievalFailureCodeCounts == nil {
			return nil, invalidMemoryToolRouteReport(memoryToolRouteInvalidIncompleteRetrieval)
		}
		wire.RouteFailureCategoryCounts = &value.RouteFailureCategoryCounts
		wire.RetrievalIncompleteCount = &value.RetrievalIncompleteCount
		wire.RetrievalFailureCodeCounts = &value.RetrievalFailureCodeCounts
	} else if value.RetrievalIncompleteCount != 0 ||
		value.RetrievalFailureCodeCounts != nil {
		return nil, invalidMemoryToolRouteReport(memoryToolRouteInvalidIncompleteRetrieval)
	}
	return json.Marshal(wire)
}
