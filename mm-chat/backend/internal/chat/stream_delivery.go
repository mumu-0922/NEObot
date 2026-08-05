package chat

import (
	"net/http"
)

// bestEffortStreamWriter separates durable generation from SSE delivery. Once
// the client connection stops accepting bytes, later events are discarded so
// the owning handler can keep consuming the Provider stream and finalize the
// assistant message. It is intentionally single-goroutine: net/http invokes a
// handler's response writes serially here.
type bestEffortStreamWriter struct {
	base       http.ResponseWriter
	controller *http.ResponseController
	detached   bool
}

func newBestEffortStreamWriter(base http.ResponseWriter) *bestEffortStreamWriter {
	return &bestEffortStreamWriter{
		base:       base,
		controller: http.NewResponseController(base),
	}
}

func (w *bestEffortStreamWriter) Header() http.Header {
	return w.base.Header()
}

func (w *bestEffortStreamWriter) WriteHeader(statusCode int) {
	if w.detached {
		return
	}
	w.base.WriteHeader(statusCode)
}

func (w *bestEffortStreamWriter) Write(payload []byte) (int, error) {
	if w.detached {
		return len(payload), nil
	}
	written, err := w.base.Write(payload)
	if err != nil || written != len(payload) {
		w.detached = true
		return len(payload), nil
	}
	return written, nil
}

func (w *bestEffortStreamWriter) Flush() {
	if w.detached {
		return
	}
	if err := w.controller.Flush(); err != nil {
		w.detached = true
	}
}
