package ragproviders

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const minerUGatewayMaxResponseBytes = 1 << 20

var (
	minerUFilenameRE   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)
	minerUIdentifierRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	minerUStartTimeRE  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)
	minerUPollStates   = map[string]bool{
		"waiting-file": true,
		"pending":      true,
		"running":      true,
		"converting":   true,
		"done":         true,
		"failed":       true,
	}
)

type minerUAllocateWireResponse struct {
	Code *int `json:"code"`
	Data *struct {
		BatchID  *string  `json:"batch_id"`
		FileURLs []string `json:"file_urls"`
	} `json:"data"`
}

type minerUPollWireResponse struct {
	Code    *int                `json:"code"`
	Data    *minerUPollWireData `json:"data"`
	Message *string             `json:"msg"`
	TraceID *string             `json:"trace_id,omitempty"`
}

type minerUPollWireData struct {
	BatchID       *string                `json:"batch_id"`
	ExtractResult []minerUPollWireResult `json:"extract_result"`
}

type minerUPollWireResult struct {
	DataID          *string                `json:"data_id,omitempty"`
	ErrorMessage    *string                `json:"err_msg"`
	ExtractProgress *minerUExtractProgress `json:"extract_progress,omitempty"`
	FileName        *string                `json:"file_name"`
	FullZIPURL      *string                `json:"full_zip_url,omitempty"`
	State           *string                `json:"state"`
}

type minerUExtractProgress struct {
	ExtractedPages *int    `json:"extracted_pages"`
	StartTime      *string `json:"start_time"`
	TotalPages     *int    `json:"total_pages"`
}

func (gateway *ProviderGateway) AllocateMinerU(
	ctx context.Context,
	input MinerUAllocateRequest,
) (MinerUAllocation, error) {
	filename, err := normalizeMinerUFilename(input.Filename)
	if err != nil {
		return MinerUAllocation{}, err
	}
	body, err := json.Marshal(map[string]any{
		"enable_formula": true,
		"enable_table":   true,
		"files":          []map[string]string{{"name": filename}},
		"is_ocr":         true,
		"model_version":  MinerUModelVersion,
	})
	if err != nil {
		return MinerUAllocation{}, ErrProviderGatewayInvalid
	}
	credential, err := gateway.resolveCredential(ctx, providerIDMinerU)
	if err != nil {
		return MinerUAllocation{}, err
	}
	raw, err := gateway.providerJSON(
		ctx,
		http.MethodPost,
		MinerUAllocateEndpoint,
		credential,
		body,
		minerUGatewayMaxResponseBytes,
	)
	credential = ""
	if err != nil {
		return MinerUAllocation{}, err
	}
	var payload minerUAllocateWireResponse
	if decodeProviderJSON(raw, &payload, false) != nil || payload.Code == nil ||
		*payload.Code != 0 || payload.Data == nil || payload.Data.BatchID == nil ||
		!validMinerUIdentifier(*payload.Data.BatchID) || len(payload.Data.FileURLs) != 1 ||
		!validMinerUUploadURL(payload.Data.FileURLs[0]) {
		return MinerUAllocation{}, ErrProviderGatewayUpstream
	}
	return MinerUAllocation{
		BatchID:   *payload.Data.BatchID,
		Filename:  filename,
		UploadURL: payload.Data.FileURLs[0],
	}, nil
}

func (gateway *ProviderGateway) PollMinerU(
	ctx context.Context,
	input MinerUPollRequest,
) (MinerUPollResult, error) {
	batchID := input.BatchID
	if batchID != strings.TrimSpace(batchID) || !validMinerUIdentifier(batchID) {
		return MinerUPollResult{}, ErrProviderGatewayInvalid
	}
	filename, err := normalizeMinerUFilename(input.Filename)
	if err != nil {
		return MinerUPollResult{}, err
	}
	credential, err := gateway.resolveCredential(ctx, providerIDMinerU)
	if err != nil {
		return MinerUPollResult{}, err
	}
	raw, err := gateway.providerJSON(
		ctx,
		http.MethodGet,
		MinerUPollEndpointPrefix+batchID,
		credential,
		nil,
		minerUGatewayMaxResponseBytes,
	)
	credential = ""
	if err != nil {
		return MinerUPollResult{}, err
	}
	return normalizeMinerUPollResponse(raw, batchID, filename)
}

func normalizeMinerUPollResponse(
	raw []byte,
	batchID string,
	filename string,
) (MinerUPollResult, error) {
	var payload minerUPollWireResponse
	if decodeProviderJSON(raw, &payload, true) != nil || payload.Code == nil ||
		*payload.Code != 0 || payload.Message == nil || *payload.Message == "" ||
		payload.Data == nil || payload.Data.BatchID == nil ||
		*payload.Data.BatchID != batchID || len(payload.Data.ExtractResult) != 1 {
		return MinerUPollResult{}, ErrProviderGatewayUpstream
	}
	result := payload.Data.ExtractResult[0]
	if result.ErrorMessage == nil || result.FileName == nil ||
		*result.FileName != filename || result.State == nil ||
		!minerUPollStates[*result.State] ||
		(result.DataID != nil && !validMinerUIdentifier(*result.DataID)) {
		return MinerUPollResult{}, ErrProviderGatewayUpstream
	}
	if *result.State == "running" {
		if !validMinerUProgress(result.ExtractProgress) {
			return MinerUPollResult{}, ErrProviderGatewayUpstream
		}
	} else if result.ExtractProgress != nil {
		return MinerUPollResult{}, ErrProviderGatewayUpstream
	}
	response := MinerUPollResult{
		BatchID: batchID, Filename: filename, State: *result.State,
	}
	if *result.State == "done" {
		if result.FullZIPURL == nil || !validMinerUResultURL(*result.FullZIPURL) {
			return MinerUPollResult{}, ErrProviderGatewayUpstream
		}
		response.ResultURL = *result.FullZIPURL
		return response, nil
	}
	if result.FullZIPURL != nil || (*result.State == "failed" && *result.ErrorMessage == "") {
		return MinerUPollResult{}, ErrProviderGatewayUpstream
	}
	return response, nil
}

func normalizeMinerUFilename(value string) (string, error) {
	if value != strings.TrimSpace(value) || !minerUFilenameRE.MatchString(value) ||
		len([]byte(value)) > 255 {
		return "", ErrProviderGatewayInvalid
	}
	return value, nil
}

func validMinerUIdentifier(value string) bool {
	return minerUIdentifierRE.MatchString(value)
}

func validMinerUProgress(progress *minerUExtractProgress) bool {
	return progress != nil && progress.ExtractedPages != nil && progress.TotalPages != nil &&
		progress.StartTime != nil && *progress.ExtractedPages >= 0 &&
		*progress.TotalPages > 0 && *progress.ExtractedPages <= *progress.TotalPages &&
		minerUStartTimeRE.MatchString(*progress.StartTime)
}

func validMinerUUploadURL(value string) bool {
	parsed, ok := parseMinerUCapabilityURL(value, MinerUUploadHost)
	return ok && parsed.RawQuery != "" &&
		strings.HasPrefix(parsed.EscapedPath(), MinerUUploadPathPrefix) &&
		safeMinerUDynamicPath(parsed.EscapedPath())
}

func validMinerUResultURL(value string) bool {
	parsed, ok := parseMinerUCapabilityURL(value, MinerUResultHost)
	return ok && parsed.RawQuery == "" &&
		strings.HasPrefix(parsed.EscapedPath(), MinerUResultPathPrefix) &&
		strings.HasSuffix(parsed.EscapedPath(), MinerUResultPathSuffix) &&
		safeMinerUDynamicPath(parsed.EscapedPath())
}

func parseMinerUCapabilityURL(value string, hostname string) (*url.URL, bool) {
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > 4096 {
		return nil, false
	}
	for _, character := range value {
		if character < 33 || character > 126 {
			return nil, false
		}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != hostname ||
		parsed.User != nil || parsed.Fragment != "" ||
		(parsed.Port() != "" && parsed.Port() != "443") {
		return nil, false
	}
	return parsed, true
}

func safeMinerUDynamicPath(value string) bool {
	if value == "" || strings.ContainsAny(value, "%\\") {
		return false
	}
	segments := strings.Split(value, "/")
	if len(segments) < 2 || segments[0] != "" {
		return false
	}
	for _, segment := range segments[1:] {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}
