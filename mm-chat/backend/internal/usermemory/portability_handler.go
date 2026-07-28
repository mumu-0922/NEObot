package usermemory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	maxEncryptedMemoryPackageBytes = 300 << 20
	maxImportMappingsBytes         = 256 << 10
	maxImportMultipartOverhead     = 1 << 20
)

type memoryExportRequest struct {
	Passphrase     string `json:"passphrase"`
	IncludeHistory bool   `json:"includeHistory"`
}

type memoryImportUpload struct {
	packageFile *os.File
	packagePath string
	passphrase  string
	mappings    ImportMappings
	planToken   string
}

func (h *Handler) handleMemoryExport(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var request memoryExportRequest
	if err := decodeJSON(r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	temporary, err := os.CreateTemp("", "mm-memory-export-*.age")
	if err != nil {
		writeServiceError(w, err)
		return
	}
	path := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(path)
	}()
	result, err := h.service.ExportMemoryPackage(
		r.Context(), temporary, request.Passphrase, request.IncludeHistory,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	info, err := temporary.Stat()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, result.Filename))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, temporary)
}

func (h *Handler) handleMemoryImport(w http.ResponseWriter, r *http.Request, confirm bool) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	upload, err := readMemoryImportMultipart(w, r, confirm)
	if err != nil {
		writeError(w, http.StatusBadRequest, "MEMORY_IMPORT_MULTIPART_INVALID", "memory import multipart payload is invalid")
		return
	}
	defer upload.close()
	if confirm {
		result, err := h.service.ConfirmMemoryImport(
			r.Context(), upload.packageFile, upload.passphrase,
			upload.mappings, upload.planToken,
		)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	result, err := h.service.DryRunMemoryImport(
		r.Context(), upload.packageFile, upload.passphrase, upload.mappings,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func readMemoryImportMultipart(
	w http.ResponseWriter,
	r *http.Request,
	confirm bool,
) (*memoryImportUpload, error) {
	r.Body = http.MaxBytesReader(
		w, r.Body, maxEncryptedMemoryPackageBytes+maxImportMultipartOverhead,
	)
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	upload := &memoryImportUpload{}
	failed := true
	defer func() {
		if failed {
			upload.close()
		}
	}()
	seen := make(map[string]struct{})
	var mappingsPayload []byte
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := part.FormName()
		if _, duplicate := seen[name]; duplicate {
			_ = part.Close()
			return nil, errors.New("duplicate memory import multipart field")
		}
		seen[name] = struct{}{}
		switch name {
		case "package":
			if part.FileName() == "" {
				_ = part.Close()
				return nil, errors.New("memory import package filename is required")
			}
			temporary, err := os.CreateTemp("", "mm-memory-import-*.age")
			if err != nil {
				_ = part.Close()
				return nil, err
			}
			upload.packageFile = temporary
			upload.packagePath = temporary.Name()
			written, copyErr := io.Copy(
				temporary,
				io.LimitReader(part, maxEncryptedMemoryPackageBytes+1),
			)
			closeErr := part.Close()
			if copyErr != nil {
				return nil, copyErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			if written == 0 || written > maxEncryptedMemoryPackageBytes {
				return nil, errors.New("memory import package is empty or too large")
			}
			if _, err := temporary.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
		case "passphrase":
			value, err := readBoundedMultipartField(part, MaxPortabilityPassphraseBytes)
			if err != nil {
				return nil, err
			}
			upload.passphrase = string(value)
		case "mappings":
			value, err := readBoundedMultipartField(part, maxImportMappingsBytes)
			if err != nil {
				return nil, err
			}
			mappingsPayload = value
		case "planToken":
			value, err := readBoundedMultipartField(part, maximumPlanTokenBytes)
			if err != nil {
				return nil, err
			}
			upload.planToken = string(value)
		default:
			_ = part.Close()
			return nil, errors.New("unknown memory import multipart field")
		}
	}
	if upload.packageFile == nil || validatePortabilityPassphrase(upload.passphrase) != nil ||
		(confirm && strings.TrimSpace(upload.planToken) == "") ||
		(!confirm && strings.TrimSpace(upload.planToken) != "") {
		return nil, errors.New("memory import multipart fields are incomplete")
	}
	if len(mappingsPayload) == 0 {
		mappingsPayload = []byte(`{"projects":{},"conversations":{}}`)
	}
	if err := decodeImportMappings(mappingsPayload, &upload.mappings); err != nil {
		return nil, err
	}
	failed = false
	return upload, nil
}

func readBoundedMultipartField(part *multipart.Part, maximum int64) ([]byte, error) {
	defer part.Close()
	value, err := io.ReadAll(io.LimitReader(part, maximum+1))
	if err != nil || int64(len(value)) > maximum {
		return nil, errors.New("memory import multipart field is too large")
	}
	return value, nil
}

func decodeImportMappings(payload []byte, target *ImportMappings) error {
	if target == nil || len(payload) == 0 || !json.Valid(payload) {
		return errors.New("memory import mappings are invalid")
	}
	if err := rejectDuplicateJSONObjectKeys(payload); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("memory import mappings contain trailing JSON")
	}
	_, err := normalizeImportMappings(*target)
	return err
}

func rejectDuplicateJSONObjectKeys(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := consumeUniqueJSONValue(decoder); err != nil {
		return err
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) || token != nil {
		return errors.New("JSON contains trailing data")
	}
	return nil
}

func consumeUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is invalid")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("JSON object contains a duplicate field")
			}
			seen[key] = struct{}{}
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("JSON object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("JSON array is not closed")
		}
	default:
		return errors.New("JSON delimiter is invalid")
	}
	return nil
}

func (u *memoryImportUpload) close() {
	if u == nil {
		return
	}
	if u.packageFile != nil {
		_ = u.packageFile.Close()
	}
	if u.packagePath != "" {
		_ = os.Remove(u.packagePath)
	}
}
