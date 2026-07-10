package teams

import "testing"

func TestNewUUIDReturnsUUID(t *testing.T) {
	id, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID() error = %v", err)
	}
	if !isUUID(id) {
		t.Fatalf("newUUID() = %q, want UUID", id)
	}
}
