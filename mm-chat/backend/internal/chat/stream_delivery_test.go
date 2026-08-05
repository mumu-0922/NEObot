package chat

import (
	"errors"
	"testing"
)

func TestBestEffortStreamWriterDetachesAfterFlushFailure(t *testing.T) {
	base := newDisconnectingResponseWriter(10)
	base.flushErr = errors.New("flush failed")
	delivery := newBestEffortStreamWriter(base)

	first := []byte("first event")
	if written, err := delivery.Write(first); err != nil || written != len(first) {
		t.Fatalf("first write = %d/%v", written, err)
	}
	delivery.Flush()

	second := []byte("second event")
	if written, err := delivery.Write(second); err != nil || written != len(second) {
		t.Fatalf("detached write = %d/%v", written, err)
	}
	if base.writes != 1 {
		t.Fatalf("base writes = %d, want 1 after detach", base.writes)
	}
}
