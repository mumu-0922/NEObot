// Independent Go 1.22 JCS fixture implementation; standard library only.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const maxSafeInteger int64 = 9007199254740991
const logicalCaseCount int64 = 24
const logicalManifest = "logical-hash-golden-v1.json"
const logicalSourcePath = "../parser_contracts/logical_hash/golden-v1.json"
const logicalFraming = "ASCII(domain-with-one-terminal-LF) || RFC8785(envelopeWithoutDomain)"

var manifests = []string{"c1-contract-profile-v1.json", "rfc8785-v1.json"}
var byteOrderMarks = [][]byte{
	{0x00, 0x00, 0xfe, 0xff},
	{0xff, 0xfe, 0x00, 0x00},
	{0xef, 0xbb, 0xbf},
	{0xfe, 0xff},
	{0xff, 0xfe},
}
var internalErrorSummary = []byte(
	`{"error":"INTERNAL_ERROR","failedCase":"","implementation":"go","status":"fail","version":"unknown"}`,
)

type conformanceError struct {
	code   string
	caseID string
}

type logicalResult struct {
	digest string
	name   string
}

type logicalSuite struct {
	rawSHA256 string
	results   []logicalResult
	suiteID   string
}

func (err *conformanceError) Error() string { return err.code }

func fail(code string) *conformanceError { return &conformanceError{code: code} }

func digest(value []byte) string {
	valueHash := sha256.Sum256(value)
	return hex.EncodeToString(valueHash[:])
}

func parseJSON(raw []byte, profile string) (any, *conformanceError) {
	for _, byteOrderMark := range byteOrderMarks {
		if bytes.HasPrefix(raw, byteOrderMark) {
			return nil, fail("BOM_FORBIDDEN")
		}
	}
	if bytes.IndexByte(raw, 0) >= 0 {
		return nil, fail("NUL_FORBIDDEN")
	}
	if !utf8.Valid(raw) {
		return nil, fail("JSON_INVALID")
	}
	if err := validateJSONStrings(raw, profile == "c1-contract-profile"); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readJSONValue(decoder, profile)
	if err != nil {
		return nil, err
	}
	if _, tokenErr := decoder.Token(); tokenErr != io.EOF {
		return nil, fail("JSON_INVALID")
	}
	return value, nil
}

func validateJSONStrings(raw []byte, rejectNUL bool) *conformanceError {
	for index := 0; index < len(raw); index++ {
		if raw[index] != '"' {
			continue
		}
		index++
		closed := false
		for index < len(raw) {
			character := raw[index]
			if character == '"' {
				closed = true
				break
			}
			if character < 0x20 {
				return fail("JSON_INVALID")
			}
			if character != '\\' {
				_, width := utf8.DecodeRune(raw[index:])
				index += width
				continue
			}
			index++
			if index >= len(raw) {
				return fail("JSON_INVALID")
			}
			if raw[index] != 'u' {
				if !strings.ContainsRune(`"\/bfnrt`, rune(raw[index])) {
					return fail("JSON_INVALID")
				}
				index++
				continue
			}
			codeUnit, next, err := parseCodeUnit(raw, index+1)
			if err != nil {
				return err
			}
			if codeUnit == 0 && rejectNUL {
				return fail("NUL_FORBIDDEN")
			}
			if 0xd800 <= codeUnit && codeUnit <= 0xdbff {
				if next+2 > len(raw) || raw[next] != '\\' || raw[next+1] != 'u' {
					return fail("SURROGATE_FORBIDDEN")
				}
				low, afterLow, lowErr := parseCodeUnit(raw, next+2)
				if lowErr != nil || low < 0xdc00 || low > 0xdfff {
					return fail("SURROGATE_FORBIDDEN")
				}
				index = afterLow
				continue
			}
			if 0xdc00 <= codeUnit && codeUnit <= 0xdfff {
				return fail("SURROGATE_FORBIDDEN")
			}
			index = next
		}
		if !closed {
			return fail("JSON_INVALID")
		}
	}
	return nil
}

func parseCodeUnit(raw []byte, start int) (uint16, int, *conformanceError) {
	if start+4 > len(raw) {
		return 0, start, fail("JSON_INVALID")
	}
	value, err := strconv.ParseUint(string(raw[start:start+4]), 16, 16)
	if err != nil {
		return 0, start, fail("JSON_INVALID")
	}
	return uint16(value), start + 4, nil
}

func readJSONValue(decoder *json.Decoder, profile string) (any, *conformanceError) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fail("JSON_INVALID")
	}
	switch typed := token.(type) {
	case nil, bool, string:
		return typed, nil
	case json.Number:
		return parseJSONNumber(string(typed), profile)
	case json.Delim:
		switch typed {
		case '[':
			values := make([]any, 0)
			for decoder.More() {
				value, valueErr := readJSONValue(decoder, profile)
				if valueErr != nil {
					return nil, valueErr
				}
				values = append(values, value)
			}
			end, endErr := decoder.Token()
			if endErr != nil || end != json.Delim(']') {
				return nil, fail("JSON_INVALID")
			}
			return values, nil
		case '{':
			values := make(map[string]any)
			for decoder.More() {
				keyToken, keyErr := decoder.Token()
				key, ok := keyToken.(string)
				if keyErr != nil || !ok {
					return nil, fail("JSON_INVALID")
				}
				if _, exists := values[key]; exists {
					return nil, fail("DUPLICATE_KEY")
				}
				value, valueErr := readJSONValue(decoder, profile)
				if valueErr != nil {
					return nil, valueErr
				}
				values[key] = value
			}
			end, endErr := decoder.Token()
			if endErr != nil || end != json.Delim('}') {
				return nil, fail("JSON_INVALID")
			}
			return values, nil
		}
	}
	return nil, fail("JSON_INVALID")
}

func parseJSONNumber(token string, profile string) (any, *conformanceError) {
	if profile == "c1-contract-profile" {
		if strings.ContainsAny(token, ".eE") {
			return nil, fail("FLOAT_FORBIDDEN")
		}
		value, err := strconv.ParseInt(token, 10, 64)
		if err != nil || value < -maxSafeInteger || value > maxSafeInteger {
			return nil, fail("UNSAFE_INTEGER")
		}
		return value, nil
	}
	value, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return nil, fail("JSON_INVALID")
	}
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, fail("NON_FINITE")
	}
	return value, nil
}

func serializeString(value string) ([]byte, *conformanceError) {
	if !utf8.ValidString(value) {
		return nil, fail("SURROGATE_FORBIDDEN")
	}
	var output bytes.Buffer
	output.WriteByte('"')
	for _, character := range value {
		switch character {
		case '\b':
			output.WriteString(`\b`)
		case '\t':
			output.WriteString(`\t`)
		case '\n':
			output.WriteString(`\n`)
		case '\f':
			output.WriteString(`\f`)
		case '\r':
			output.WriteString(`\r`)
		case '"':
			output.WriteString(`\"`)
		case '\\':
			output.WriteString(`\\`)
		default:
			if character <= 0x1f {
				fmt.Fprintf(&output, `\u%04x`, character)
			} else {
				output.WriteRune(character)
			}
		}
	}
	output.WriteByte('"')
	return output.Bytes(), nil
}

func serializeFloat(value float64) ([]byte, *conformanceError) {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, fail("NON_FINITE")
	}
	if value == 0 {
		return []byte("0"), nil
	}
	format := byte('f')
	absolute := math.Abs(value)
	if absolute != 0 && (absolute < 1e-6 || absolute >= 1e21) {
		format = 'e'
	}
	encoded := strconv.AppendFloat(nil, value, format, -1, 64)
	if format == 'e' {
		exponent := bytes.IndexByte(encoded, 'e') + 1
		if exponent < len(encoded) && (encoded[exponent] == '+' || encoded[exponent] == '-') {
			exponent++
		}
		if exponent < len(encoded)-1 && encoded[exponent] == '0' {
			encoded = append(encoded[:exponent], encoded[exponent+1:]...)
		}
	}
	return encoded, nil
}

func canonicalize(value any) ([]byte, *conformanceError) {
	switch typed := value.(type) {
	case nil:
		return []byte("null"), nil
	case bool:
		if typed {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case int64:
		if typed < -maxSafeInteger || typed > maxSafeInteger {
			return nil, fail("UNSAFE_INTEGER")
		}
		return []byte(strconv.FormatInt(typed, 10)), nil
	case int:
		return canonicalize(int64(typed))
	case float64:
		return serializeFloat(typed)
	case string:
		return serializeString(typed)
	case []any:
		var output bytes.Buffer
		output.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				output.WriteByte(',')
			}
			member, err := canonicalize(item)
			if err != nil {
				return nil, err
			}
			output.Write(member)
		}
		output.WriteByte(']')
		return output.Bytes(), nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(left, right int) bool {
			return compareUTF16(keys[left], keys[right]) < 0
		})
		var output bytes.Buffer
		output.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				output.WriteByte(',')
			}
			encodedKey, keyErr := serializeString(key)
			if keyErr != nil {
				return nil, keyErr
			}
			encodedValue, valueErr := canonicalize(typed[key])
			if valueErr != nil {
				return nil, valueErr
			}
			output.Write(encodedKey)
			output.WriteByte(':')
			output.Write(encodedValue)
		}
		output.WriteByte('}')
		return output.Bytes(), nil
	}
	return nil, fail("JSON_INVALID")
}

func compareUTF16(left string, right string) int {
	leftUnits := utf16.Encode([]rune(left))
	rightUnits := utf16.Encode([]rune(right))
	limit := len(leftUnits)
	if len(rightUnits) < limit {
		limit = len(rightUnits)
	}
	for index := 0; index < limit; index++ {
		if leftUnits[index] < rightUnits[index] {
			return -1
		}
		if leftUnits[index] > rightUnits[index] {
			return 1
		}
	}
	if len(leftUnits) < len(rightUnits) {
		return -1
	}
	if len(leftUnits) > len(rightUnits) {
		return 1
	}
	return 0
}

func expectObject(value any, fields ...string) (map[string]any, *conformanceError) {
	object, ok := value.(map[string]any)
	if !ok || len(object) != len(fields) {
		return nil, fail("MANIFEST_INVALID")
	}
	for _, field := range fields {
		if _, exists := object[field]; !exists {
			return nil, fail("MANIFEST_INVALID")
		}
	}
	return object, nil
}

func expectText(value any, asciiOnly bool) (string, *conformanceError) {
	text, ok := value.(string)
	if !ok || text == "" {
		return "", fail("MANIFEST_INVALID")
	}
	if asciiOnly {
		for _, character := range []byte(text) {
			if character < 0x20 || character > 0x7e {
				return "", fail("MANIFEST_INVALID")
			}
		}
	}
	return text, nil
}

func expectSHA(value any) (string, *conformanceError) {
	text, err := expectText(value, true)
	if err != nil || len(text) != 64 {
		return "", fail("MANIFEST_INVALID")
	}
	for _, character := range text {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return "", fail("MANIFEST_INVALID")
		}
	}
	return text, nil
}

func decodeHex(value any) ([]byte, *conformanceError) {
	text, ok := value.(string)
	if !ok || len(text)%2 != 0 {
		return nil, fail("MANIFEST_INVALID")
	}
	decoded, err := hex.DecodeString(text)
	if err != nil {
		return nil, fail("MANIFEST_INVALID")
	}
	return decoded, nil
}

func validateCase(value any, profile string) (map[string]any, *conformanceError) {
	base, ok := value.(map[string]any)
	if !ok {
		return nil, fail("MANIFEST_INVALID")
	}
	kind, kindErr := expectText(base["kind"], true)
	expectation, expectErr := expectText(base["expect"], true)
	if kindErr != nil || expectErr != nil {
		return nil, fail("MANIFEST_INVALID")
	}
	fields := []string{"caseId", "kind", "expect", "inputSha256"}
	var encodedInput any
	if kind == "json" {
		fields = append(fields, "inputHex")
		encodedInput = base["inputHex"]
	} else if kind == "ieee754" && profile == "rfc8785" {
		fields = append(fields, "ieee754Hex")
		encodedInput = base["ieee754Hex"]
	} else {
		return nil, fail("MANIFEST_INVALID")
	}
	if expectation == "accept" {
		fields = append(fields, "expectedHex", "expectedSha256")
	} else if expectation == "reject" {
		fields = append(fields, "reasonCode")
	} else {
		return nil, fail("MANIFEST_INVALID")
	}
	testCase, objectErr := expectObject(base, fields...)
	if objectErr != nil {
		return nil, objectErr
	}
	if _, err := expectText(testCase["caseId"], true); err != nil {
		return nil, err
	}
	input, inputErr := decodeHex(encodedInput)
	inputHash, hashErr := expectSHA(testCase["inputSha256"])
	if inputErr != nil || hashErr != nil || (kind == "ieee754" && len(input) != 8) {
		return nil, fail("MANIFEST_INVALID")
	}
	if digest(input) != inputHash {
		return nil, fail("FIXTURE_HASH_MISMATCH")
	}
	if expectation == "accept" {
		expected, expectedErr := decodeHex(testCase["expectedHex"])
		expectedHash, expectedHashErr := expectSHA(testCase["expectedSha256"])
		if expectedErr != nil || expectedHashErr != nil {
			return nil, fail("MANIFEST_INVALID")
		}
		if digest(expected) != expectedHash {
			return nil, fail("FIXTURE_HASH_MISMATCH")
		}
	} else if _, err := expectText(testCase["reasonCode"], true); err != nil {
		return nil, err
	}
	return testCase, nil
}

func loadManifest(path string) (map[string]any, *conformanceError) {
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, fail("MANIFEST_INVALID")
	}
	parsed, parseErr := parseJSON(raw, "c1-contract-profile")
	if parseErr != nil {
		return nil, parseErr
	}
	canonical, canonicalErr := canonicalize(parsed)
	if canonicalErr != nil || !bytes.Equal(canonical, raw) {
		return nil, fail("MANIFEST_NOT_CANONICAL")
	}
	manifest, objectErr := expectObject(
		parsed,
		"schemaVersion", "suiteId", "profile", "provenance", "fixtureSetSha256", "cases",
	)
	if objectErr != nil || manifest["schemaVersion"] != "mm-chat.jcs-vector-manifest.v1" {
		return nil, fail("MANIFEST_INVALID")
	}
	profile, profileErr := expectText(manifest["profile"], true)
	if profileErr != nil || (profile != "c1-contract-profile" && profile != "rfc8785") {
		return nil, fail("MANIFEST_INVALID")
	}
	if _, err := expectText(manifest["suiteId"], true); err != nil {
		return nil, err
	}
	provenance, provenanceErr := expectObject(
		manifest["provenance"],
		"source", "sourceUrl", "revision", "materialSha256", "license", "licenseFile",
	)
	if provenanceErr != nil {
		return nil, provenanceErr
	}
	for _, field := range []string{"source", "sourceUrl", "revision", "license", "licenseFile"} {
		if _, err := expectText(provenance[field], false); err != nil {
			return nil, err
		}
	}
	cases, ok := manifest["cases"].([]any)
	if !ok || len(cases) == 0 {
		return nil, fail("MANIFEST_INVALID")
	}
	for _, testCase := range cases {
		if _, err := validateCase(testCase, profile); err != nil {
			return nil, err
		}
	}
	caseBytes, caseErr := canonicalize(cases)
	fixtureHash, fixtureErr := expectSHA(manifest["fixtureSetSha256"])
	materialHash, materialErr := expectSHA(provenance["materialSha256"])
	if caseErr != nil || fixtureErr != nil || materialErr != nil {
		return nil, fail("MANIFEST_INVALID")
	}
	actualHash := digest(caseBytes)
	if actualHash != fixtureHash {
		return nil, fail("FIXTURE_SET_HASH_MISMATCH")
	}
	if actualHash != materialHash {
		return nil, fail("PROVENANCE_HASH_MISMATCH")
	}
	return manifest, nil
}

func expectLogicalName(value any) (string, *conformanceError) {
	name, err := expectText(value, true)
	if err != nil {
		return "", fail("LOGICAL_GOLDEN_INVALID")
	}
	for _, character := range []byte(name) {
		if character < 0x21 || character > 0x7e {
			return "", fail("LOGICAL_GOLDEN_INVALID")
		}
	}
	return name, nil
}

func expectLogicalDomain(value any) ([]byte, *conformanceError) {
	domain, ok := value.(string)
	if !ok || len(domain) < 2 || domain[len(domain)-1] != '\n' {
		return nil, fail("LOGICAL_GOLDEN_INVALID")
	}
	for _, character := range []byte(domain[:len(domain)-1]) {
		if character < 0x21 || character > 0x7e {
			return nil, fail("LOGICAL_GOLDEN_INVALID")
		}
	}
	return []byte(domain), nil
}

func loadLogicalSuite(fixtures string) (*logicalSuite, *conformanceError) { //nolint:gocyclo
	manifestRaw, readErr := os.ReadFile(filepath.Join(fixtures, logicalManifest))
	if readErr != nil {
		return nil, fail("MANIFEST_INVALID")
	}
	parsedManifest, parseErr := parseJSON(manifestRaw, "c1-contract-profile")
	if parseErr != nil {
		return nil, parseErr
	}
	canonicalManifest, canonicalErr := canonicalize(parsedManifest)
	if canonicalErr != nil || !bytes.Equal(canonicalManifest, manifestRaw) {
		return nil, fail("MANIFEST_NOT_CANONICAL")
	}
	manifest, objectErr := expectObject(
		parsedManifest,
		"caseCount", "profile", "provenance", "schemaVersion", "suiteId",
	)
	if objectErr != nil {
		return nil, fail("MANIFEST_INVALID")
	}
	caseCount, countOK := manifest["caseCount"].(int64)
	if !countOK || caseCount != logicalCaseCount ||
		manifest["schemaVersion"] != "mm-chat.jcs-logical-hash-manifest.v1" ||
		manifest["suiteId"] != "logical-hash-golden-v1" ||
		manifest["profile"] != "c1-contract-profile" {
		return nil, fail("MANIFEST_INVALID")
	}
	provenance, provenanceErr := expectObject(
		manifest["provenance"],
		"license", "licenseFile", "materialSha256", "revision", "source", "sourcePath",
	)
	if provenanceErr != nil {
		return nil, provenanceErr
	}
	for _, field := range []string{"license", "licenseFile", "revision", "source"} {
		if _, err := expectText(provenance[field], false); err != nil {
			return nil, err
		}
	}
	if provenance["sourcePath"] != logicalSourcePath || provenance["licenseFile"] != "README.md" {
		return nil, fail("MANIFEST_INVALID")
	}

	sourceRaw, sourceErr := os.ReadFile(filepath.Join(fixtures, logicalSourcePath))
	if sourceErr != nil {
		return nil, fail("LOGICAL_GOLDEN_INVALID")
	}
	sourceHash := digest(sourceRaw)
	materialHash, materialErr := expectSHA(provenance["materialSha256"])
	if materialErr != nil || sourceHash != materialHash {
		return nil, fail("PROVENANCE_HASH_MISMATCH")
	}
	parsedGolden, goldenParseErr := parseJSON(sourceRaw, "c1-contract-profile")
	if goldenParseErr != nil {
		return nil, goldenParseErr
	}
	canonicalGolden, goldenCanonicalErr := canonicalize(parsedGolden)
	if goldenCanonicalErr != nil || !bytes.Equal(canonicalGolden, sourceRaw) {
		return nil, fail("LOGICAL_GOLDEN_NOT_CANONICAL")
	}
	golden, goldenObjectErr := expectObject(parsedGolden, "algorithm", "framing", "vectors")
	if goldenObjectErr != nil {
		return nil, fail("LOGICAL_GOLDEN_INVALID")
	}
	vectors, vectorsOK := golden["vectors"].([]any)
	if !vectorsOK || int64(len(vectors)) != logicalCaseCount ||
		golden["algorithm"] != "sha-256" || golden["framing"] != logicalFraming {
		return nil, fail("LOGICAL_GOLDEN_INVALID")
	}

	names := make(map[string]struct{}, len(vectors))
	results := make([]logicalResult, 0, len(vectors))
	for _, rawVector := range vectors {
		vector, vectorErr := expectObject(
			rawVector,
			"domain", "envelopeWithoutDomain", "expectedSha256", "name",
		)
		if vectorErr != nil {
			return nil, fail("LOGICAL_GOLDEN_INVALID")
		}
		name, nameErr := expectLogicalName(vector["name"])
		if _, exists := names[name]; nameErr != nil || exists {
			return nil, fail("LOGICAL_GOLDEN_INVALID")
		}
		names[name] = struct{}{}
		domain, domainErr := expectLogicalDomain(vector["domain"])
		expected, expectedErr := expectSHA(vector["expectedSha256"])
		envelope, envelopeOK := vector["envelopeWithoutDomain"].(map[string]any)
		if domainErr != nil || expectedErr != nil || !envelopeOK || envelope["envelopeKind"] != name {
			return nil, fail("LOGICAL_GOLDEN_INVALID")
		}
		if _, hasDomain := envelope["domain"]; hasDomain {
			return nil, fail("LOGICAL_GOLDEN_INVALID")
		}
		canonicalEnvelope, envelopeErr := canonicalize(envelope)
		if envelopeErr != nil {
			return nil, envelopeErr
		}
		framed := make([]byte, 0, len(domain)+len(canonicalEnvelope))
		framed = append(framed, domain...)
		framed = append(framed, canonicalEnvelope...)
		actual := digest(framed)
		if actual != expected {
			return nil, &conformanceError{code: "LOGICAL_HASH_MISMATCH", caseID: name}
		}
		results = append(results, logicalResult{digest: actual, name: name})
	}
	return &logicalSuite{rawSHA256: sourceHash, results: results, suiteID: "logical-hash-golden-v1"}, nil
}

func executeCase(testCase map[string]any, profile string) (string, string, *conformanceError) {
	caseID, _ := expectText(testCase["caseId"], true)
	kind, _ := expectText(testCase["kind"], true)
	expectation, _ := expectText(testCase["expect"], true)
	var canonical []byte
	var operationErr *conformanceError
	if kind == "json" {
		raw, _ := decodeHex(testCase["inputHex"])
		value, parseErr := parseJSON(raw, profile)
		if parseErr != nil {
			operationErr = parseErr
		} else {
			canonical, operationErr = canonicalize(value)
			if operationErr == nil && profile == "c1-contract-profile" && !bytes.Equal(canonical, raw) {
				operationErr = fail("NON_CANONICAL")
			}
		}
	} else {
		raw, _ := decodeHex(testCase["ieee754Hex"])
		bits, _ := strconv.ParseUint(hex.EncodeToString(raw), 16, 64)
		canonical, operationErr = serializeFloat(math.Float64frombits(bits))
	}
	if operationErr != nil {
		if expectation == "reject" && operationErr.code == testCase["reasonCode"] {
			return "reject", operationErr.code, nil
		}
		operationErr.caseID = caseID
		return "", "", operationErr
	}
	if expectation == "reject" {
		return "", "", &conformanceError{code: "UNEXPECTED_ACCEPT", caseID: caseID}
	}
	expected, _ := decodeHex(testCase["expectedHex"])
	if !bytes.Equal(canonical, expected) {
		return "", "", &conformanceError{code: "CANONICAL_BYTES_MISMATCH", caseID: caseID}
	}
	canonicalHash := digest(canonical)
	if canonicalHash != testCase["expectedSha256"] {
		return "", "", &conformanceError{code: "CANONICAL_HASH_MISMATCH", caseID: caseID}
	}
	return "accept", canonicalHash, nil
}

func run(fixtures string) (map[string]any, *conformanceError) {
	var transcript bytes.Buffer
	caseCount := 0
	for _, filename := range manifests {
		manifest, manifestErr := loadManifest(filepath.Join(fixtures, filename))
		if manifestErr != nil {
			return nil, manifestErr
		}
		profile, _ := expectText(manifest["profile"], true)
		suiteID, _ := expectText(manifest["suiteId"], true)
		cases := manifest["cases"].([]any)
		for _, value := range cases {
			testCase := value.(map[string]any)
			outcome, result, caseErr := executeCase(testCase, profile)
			if caseErr != nil {
				return nil, caseErr
			}
			caseID, _ := expectText(testCase["caseId"], true)
			fmt.Fprintf(&transcript, "%s\x00%s\x00%s\x00%s\n", suiteID, caseID, outcome, result)
			caseCount++
		}
	}
	logical, logicalErr := loadLogicalSuite(fixtures)
	if logicalErr != nil {
		return nil, logicalErr
	}
	for _, result := range logical.results {
		fmt.Fprintf(
			&transcript,
			"%s\x00%s\x00accept\x00%s\n",
			logical.suiteID,
			result.name,
			result.digest,
		)
		caseCount++
	}
	return map[string]any{
		"caseCount":              int64(caseCount),
		"implementation":         "go",
		"logicalGoldenRawSha256": logical.rawSHA256,
		"resultSha256":           digest(transcript.Bytes()),
		"status":                 "pass",
		"suiteCount":             int64(len(manifests) + 1),
		"version":                strings.TrimPrefix(runtime.Version(), "go"),
	}, nil
}

func writeSummary(summary map[string]any) {
	encoded, err := canonicalize(summary)
	if err != nil {
		encoded = internalErrorSummary
	}
	for _, character := range encoded {
		if character > 0x7f {
			encoded = internalErrorSummary
			break
		}
	}
	os.Stdout.Write(encoded)      //nolint:errcheck -- deterministic best-effort CLI output
	os.Stdout.Write([]byte{'\n'}) //nolint:errcheck -- deterministic best-effort CLI output
}

func main() {
	version := strings.TrimPrefix(runtime.Version(), "go")
	if len(os.Args) != 3 || os.Args[1] != "--fixtures" {
		writeSummary(map[string]any{
			"error":          "CLI_ARGUMENT_INVALID",
			"failedCase":     "",
			"implementation": "go",
			"status":         "fail",
			"version":        version,
		})
		os.Exit(2)
	}
	summary, err := run(os.Args[2])
	if err != nil {
		writeSummary(map[string]any{
			"error":          err.code,
			"failedCase":     err.caseID,
			"implementation": "go",
			"status":         "fail",
			"version":        version,
		})
		os.Exit(1)
	}
	writeSummary(summary)
}
