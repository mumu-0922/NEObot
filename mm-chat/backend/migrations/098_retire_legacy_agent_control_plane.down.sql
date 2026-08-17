-- Irreversible by design: migration 098 removes an unused control plane only
-- after fail-closed emptiness checks. Rollback restores source/database backups;
-- it never recreates obsolete execution authority or synthetic state.
DO $retire_legacy_agent_control_plane_down$
BEGIN
  NULL;
END
$retire_legacy_agent_control_plane_down$;
