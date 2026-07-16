-- Revert G7.5L parse source metadata grant fix.

REVOKE SELECT ON files FROM rag_projection_owner;
