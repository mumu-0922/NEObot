-- G7.5L parse source metadata grant fix.
--
-- The SECURITY DEFINER function from 016 reads `files` while owned by
-- `rag_projection_owner`. Fresh live parse promotion caught that the owner had
-- all Knowledge projection reads but not file-metadata SELECT, so metadata
-- lookup failed before the Go source-object gateway could be called.

GRANT SELECT ON files TO rag_projection_owner;
