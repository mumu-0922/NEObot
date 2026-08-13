package agentrunner

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

const maxArtifactHeaderBytes = 1024

type artifactFrame struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
}

type ArtifactIntakeListener struct {
	listener  *net.UnixListener
	broker    *ArtifactBroker
	attempt   AttemptIdentity
	isCurrent func(AttemptIdentity) bool
	closeOnce sync.Once
}

func StartArtifactIntake(socketPath string, broker *ArtifactBroker, attempt AttemptIdentity,
	isCurrent func(AttemptIdentity) bool,
) (*ArtifactIntakeListener, error) {
	if broker == nil || isCurrent == nil || !validID(attempt.RunID, "run") ||
		!validID(attempt.StepID, "step") || !validID(attempt.AttemptID, "attempt") ||
		attempt.LeaseGeneration < 1 || !secureAbsolutePath(socketPath) {
		return nil, ErrInvalidInput
	}
	parent := filepath.Dir(socketPath)
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o711 {
		return nil, ErrInvalidInput
	}
	if removeErr := os.Remove(socketPath); removeErr != nil && !os.IsNotExist(removeErr) {
		return nil, ErrRuntimeUnavailable
	}
	address := &net.UnixAddr{Name: socketPath, Net: "unix"}
	listener, err := net.ListenUnix("unix", address)
	if err != nil {
		return nil, ErrRuntimeUnavailable
	}
	if err := os.Chmod(socketPath, 0o622); err != nil {
		listener.Close()
		_ = os.Remove(socketPath)
		return nil, ErrRuntimeUnavailable
	}
	intake := &ArtifactIntakeListener{listener: listener, broker: broker, attempt: attempt, isCurrent: isCurrent}
	go intake.serve()
	return intake, nil
}

func (intake *ArtifactIntakeListener) Close() error {
	if intake == nil {
		return nil
	}
	var closeErr error
	intake.closeOnce.Do(func() {
		closeErr = intake.listener.Close()
		if removeErr := os.Remove(intake.listener.Addr().String()); closeErr == nil && removeErr != nil && !os.IsNotExist(removeErr) {
			closeErr = removeErr
		}
	})
	return closeErr
}

func (intake *ArtifactIntakeListener) serve() {
	for {
		connection, err := intake.listener.AcceptUnix()
		if err != nil {
			return
		}
		go intake.handle(connection)
	}
}

func (intake *ArtifactIntakeListener) handle(connection *net.UnixConn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	reader := bufio.NewReaderSize(connection, 32<<10)
	headerSizeBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, headerSizeBytes); err != nil {
		return
	}
	headerSize := binary.BigEndian.Uint32(headerSizeBytes)
	if headerSize == 0 || headerSize > maxArtifactHeaderBytes {
		return
	}
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(reader, header); err != nil {
		return
	}
	var frame artifactFrame
	if strictjson.Decode(header, maxArtifactHeaderBytes, &frame) != nil || !intake.isCurrent(intake.attempt) {
		return
	}
	upload, err := intake.broker.Begin(intake.attempt, ArtifactMetadata{Name: frame.Name, MediaType: frame.MediaType, Size: frame.Size})
	if err != nil {
		return
	}
	defer upload.Abort()
	if _, err := io.CopyN(upload, reader, frame.Size); err != nil || !intake.isCurrent(intake.attempt) {
		return
	}
	receipt, err := upload.Finalize(intake.attempt)
	if err != nil {
		return
	}
	response, err := json.Marshal(receipt)
	if err != nil || len(response) > maxArtifactHeaderBytes {
		return
	}
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(response)))
	_, _ = connection.Write(append(length, response...))
}

func writeArtifactFrame(ctx context.Context, socketPath string, frame artifactFrame, body []byte) (ArtifactReceipt, error) {
	if int64(len(body)) != frame.Size {
		return ArtifactReceipt{}, ErrInvalidInput
	}
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return ArtifactReceipt{}, err
	}
	defer connection.Close()
	header, _ := json.Marshal(frame)
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(header)))
	if _, err := connection.Write(append(append(length, header...), body...)); err != nil {
		return ArtifactReceipt{}, err
	}
	responseLength := make([]byte, 4)
	if _, err := io.ReadFull(connection, responseLength); err != nil {
		return ArtifactReceipt{}, err
	}
	size := binary.BigEndian.Uint32(responseLength)
	if size == 0 || size > maxArtifactHeaderBytes {
		return ArtifactReceipt{}, ErrRuntimeUnavailable
	}
	response := make([]byte, size)
	if _, err := io.ReadFull(connection, response); err != nil {
		return ArtifactReceipt{}, err
	}
	var receipt ArtifactReceipt
	if err := strictjson.Decode(response, maxArtifactHeaderBytes, &receipt); err != nil {
		return ArtifactReceipt{}, ErrRuntimeUnavailable
	}
	return receipt, nil
}
