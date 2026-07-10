package teams

import (
	"strings"
	"testing"
)

func TestCursorCodecEncodeDecodeAndBindings(t *testing.T) {
	codec, err := NewCursorCodec(CursorKeyring{
		ActiveKeyID: "cursor-active",
		Keys: map[string][]byte{
			"cursor-active": repeatedKey('a'),
		},
	})
	if err != nil {
		t.Fatalf("NewCursorCodec() error = %v", err)
	}

	encoded, err := codec.Encode(Cursor{
		Resource: cursorResourceTeams,
		UserID:   "11111111-1111-4111-8111-111111111111",
		Sort:     cursorSortCreatedDesc,
		Values: []string{
			"2026-07-10T12:00:00Z",
			"22222222-2222-4222-8222-222222222222",
		},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded, err := codec.Decode(encoded, CursorContext{
		Resource: cursorResourceTeams,
		UserID:   "11111111-1111-4111-8111-111111111111",
		Sort:     cursorSortCreatedDesc,
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded.KeyID != "cursor-active" || len(decoded.Values) != 2 {
		t.Fatalf("Decode() = %#v", decoded)
	}

	if _, err := codec.Decode(encoded, CursorContext{
		Resource: cursorResourceTeams,
		UserID:   "33333333-3333-4333-8333-333333333333",
		Sort:     cursorSortCreatedDesc,
	}); err == nil {
		t.Fatal("Decode() with mismatched user error = nil, want error")
	}
}

func TestCursorCodecRejectsTamperingAndOversize(t *testing.T) {
	codec, err := NewCursorCodec(CursorKeyring{
		ActiveKeyID: "cursor-active",
		Keys: map[string][]byte{
			"cursor-active": repeatedKey('b'),
		},
	})
	if err != nil {
		t.Fatalf("NewCursorCodec() error = %v", err)
	}

	encoded, err := codec.Encode(Cursor{
		Resource: cursorResourceTeamInvites,
		UserID:   "11111111-1111-4111-8111-111111111111",
		TeamID:   "22222222-2222-4222-8222-222222222222",
		Sort:     cursorSortCreatedDesc,
		Values: []string{
			"2026-07-10T12:00:00Z",
			"33333333-3333-4333-8333-333333333333",
		},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	parts := strings.Split(encoded, ".")
	if len(parts) != 2 {
		t.Fatalf("cursor split length = %d", len(parts))
	}
	tampered := parts[0] + "." + parts[1][:len(parts[1])-1] + "A"
	if _, err := codec.Decode(tampered, CursorContext{
		Resource: cursorResourceTeamInvites,
		UserID:   "11111111-1111-4111-8111-111111111111",
		TeamID:   "22222222-2222-4222-8222-222222222222",
		Sort:     cursorSortCreatedDesc,
	}); err == nil {
		t.Fatal("Decode() tampered error = nil, want error")
	}

	if _, err := codec.Decode(strings.Repeat("a", maximumCursorBytes+1), CursorContext{
		Resource: cursorResourceTeams,
		UserID:   "11111111-1111-4111-8111-111111111111",
		Sort:     cursorSortCreatedDesc,
	}); err == nil {
		t.Fatal("Decode() oversize error = nil, want error")
	}
}

func TestCursorCodecSupportsVerifyOnlyRotation(t *testing.T) {
	oldCodec, err := NewCursorCodec(CursorKeyring{
		ActiveKeyID: "old",
		Keys: map[string][]byte{
			"old": repeatedKey('c'),
		},
	})
	if err != nil {
		t.Fatalf("NewCursorCodec(old) error = %v", err)
	}
	rotatedCodec, err := NewCursorCodec(CursorKeyring{
		ActiveKeyID: "new",
		Keys: map[string][]byte{
			"new": repeatedKey('d'),
			"old": repeatedKey('c'),
		},
	})
	if err != nil {
		t.Fatalf("NewCursorCodec(rotated) error = %v", err)
	}

	encoded, err := oldCodec.Encode(Cursor{
		Resource: cursorResourceTeamMembers,
		UserID:   "11111111-1111-4111-8111-111111111111",
		TeamID:   "22222222-2222-4222-8222-222222222222",
		Sort:     cursorSortMemberAsc,
		Values: []string{
			"2026-07-10T12:00:00Z",
			"33333333-3333-4333-8333-333333333333",
		},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded, err := rotatedCodec.Decode(encoded, CursorContext{
		Resource: cursorResourceTeamMembers,
		UserID:   "11111111-1111-4111-8111-111111111111",
		TeamID:   "22222222-2222-4222-8222-222222222222",
		Sort:     cursorSortMemberAsc,
	})
	if err != nil {
		t.Fatalf("Decode() rotated error = %v", err)
	}
	if decoded.KeyID != "old" {
		t.Fatalf("Decode() key id = %q, want old", decoded.KeyID)
	}
}

func TestCursorKeyringRejectsNormalizedDuplicateIDs(t *testing.T) {
	_, err := NewCursorCodec(CursorKeyring{
		ActiveKeyID: "active",
		Keys: map[string][]byte{
			"active":   repeatedKey('a'),
			" active ": repeatedKey('b'),
		},
	})
	if err == nil {
		t.Fatal("NewCursorCodec() accepted duplicate normalized key ids")
	}
}
