package migration

import "testing"

func TestPhase15RAGParseSourceMetadataGrantFixContract(t *testing.T) {
	up := readPhase15SQL(t, "019_rag_parse_source_metadata_grant_fix.up.sql")
	down := readPhase15SQL(t, "019_rag_parse_source_metadata_grant_fix.down.sql")

	assertPhase15Fragments(t, up,
		"019 must let the 016 security-definer owner read canonical file metadata",
		"grant select on files to rag_projection_owner")
	assertPhase15Fragments(t, down,
		"019 rollback must remove only the file metadata grant it adds",
		"revoke select on files from rag_projection_owner")
}
