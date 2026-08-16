#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-draft-worker-pg17-$RANDOM-$$"
restore_container_name="${container_name}-restore"
clean_container_name="${container_name}-clean"
database_name="neo_chat_agent_draft_worker_drill"
database_user="postgres"
database_password="agent-draft-worker-drill-$(openssl rand -hex 16)"
runtime_user="agent_draft_learning_worker_app"
runtime_password="agent-draft-worker-runtime-$(openssl rand -hex 16)"
activation_id="activation_21c5000000000002"
runner_id="neo-runner-primary"
runner_snapshot_fingerprint="sha256:6666666666666666666666666666666666666666666666666666666666666666"
workspace_snapshot_id="workspace_snapshot_21c5000000000002"
workspace_fingerprint="sha256:7777777777777777777777777777777777777777777777777777777777777777"
isolation_suite_fingerprint="sha256:8888888888888888888888888888888888888888888888888888888888888888"
evaluation_suite_fingerprint="sha256:9999999999999999999999999999999999999999999999999999999999999999"
plan_fingerprint="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
lease_token_hash="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" "${restore_container_name}" \
    "${clean_container_name}" >/dev/null 2>&1 || true
  find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
log() { printf 'Agent Draft-learning worker PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker go openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Agent Draft-learning worker PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Agent Draft-learning worker PostgreSQL 17 drill: Docker is required" >&2
  exit 1
}
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then
  docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null
fi

start_database() {
  local name="$1"
  docker run -d --rm --name "${name}" \
    -e "POSTGRES_DB=${database_name}" -e "POSTGRES_USER=${database_user}" \
    -e "POSTGRES_PASSWORD=${database_password}" -p 127.0.0.1::5432 \
    "${postgres_image}" >/dev/null
  for _ in $(seq 1 60); do
    docker exec "${name}" pg_isready -U "${database_user}" \
      -d "${database_name}" >/dev/null 2>&1 && return
    sleep 1
  done
  return 1
}
database_url_for() {
  local name="$1" port
  port="$(docker port "${name}" 5432/tcp | sed -n '1s/.*://p')"
  printf 'postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable' \
    "${database_user}" "${database_password}" "${port}" "${database_name}"
}
psql_command() {
  local name="$1" command="$2"
  docker exec -e "PGPASSWORD=${database_password}" "${name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align \
    --username="${database_user}" --dbname="${database_name}" --command "${command}"
}
psql_runtime() {
  local command="$1"
  docker exec -e "PGPASSWORD=${runtime_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align \
    --username="${runtime_user}" --dbname="${database_name}" --command "${command}"
}
expect_runtime_denied() {
  local label="$1" command="$2"
  set +e
  psql_runtime "${command}" >"${work_dir}/${label}.log" 2>&1
  local status=$?
  set -e
  if [[ "${status}" -eq 0 ]] || ! grep -Eiq 'permission denied|must be owner' "${work_dir}/${label}.log"; then
    cat "${work_dir}/${label}.log" >&2
    echo "Agent Draft-learning worker PostgreSQL 17 drill: ${label} was not denied" >&2
    exit 1
  fi
}

log "starting disposable PostgreSQL"
start_database "${container_name}"
database_url="$(database_url_for "${container_name}")"
[[ "$(psql_command "${container_name}" 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -buildvcs=false -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" "$@"; }

log "applying and replaying migrations through schema head 097"
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 089_agent_draft_learning" "${work_dir}/fresh.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/fresh.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/fresh.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/fresh.log"
grep -Fq "up 097_chat_agent_goals" "${work_dir}/fresh.log"
[[ "$(psql_command "${container_name}" 'SELECT max(version) FROM schema_migrations')" == "97" ]]
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"

log "retaining two independent quarantined Drafts"
(
  cd "${backend_dir}"
  MM_CHAT_AGENT_LEARNING_RETAIN_FIXTURE=1 MM_CHAT_TEST_DATABASE_URL="${database_url}" \
    go test -count=1 -run '^TestPostgresAgentLearningDraftChecksPromotionCleanup$' \
      ./internal/agentlearning
)
mapfile -t draft_records < <(psql_command "${container_name}" "
SELECT concat_ws('|',id,user_id,draft_fingerprint,proposed_package_fingerprint,
  spec->>'runtimeBundleFingerprint',spec->>'archiveFingerprint',
  spec->>'sbomFingerprint',spec->>'name',spec->>'version',spec->>'archiveBytes')
FROM agent_learning_drafts WHERE state='quarantined'
ORDER BY id DESC LIMIT 2;")
if [[ "${#draft_records[@]}" -ne 2 ]]; then
  echo "Agent Draft-learning worker PostgreSQL 17 drill: expected two quarantined Drafts" >&2
  exit 1
fi
IFS='|' read -r target_draft target_user target_draft_fingerprint \
  target_package_fingerprint target_runtime_fingerprint target_archive_fingerprint \
  target_sbom_fingerprint target_name target_version target_archive_bytes \
  <<<"${draft_records[0]}"
IFS='|' read -r unrelated_draft _ <<<"${draft_records[1]}"

log "provisioning one immutable target and an exact function-only LOGIN"
psql_command "${container_name}" "
SELECT activation_id FROM agent_learning_worker_provision_target(
  '${activation_id}','${target_draft}','${target_user}'::uuid,
  '${target_draft_fingerprint}','${target_package_fingerprint}',
  '${target_runtime_fingerprint}','${target_archive_fingerprint}',
  '${runner_snapshot_fingerprint}','${workspace_snapshot_id}','${workspace_fingerprint}',
  '${isolation_suite_fingerprint}','${evaluation_suite_fingerprint}','${plan_fingerprint}',
  clock_timestamp()-interval '1 minute',clock_timestamp()+interval '1 hour',
  'g21.5-draft-operator','G21_5_DRAFT_APPROVED');
CREATE ROLE ${runtime_user} LOGIN INHERIT PASSWORD '${runtime_password}';
GRANT agent_learning_worker TO ${runtime_user};
DO \$\$
DECLARE inherited_roles name[];
BEGIN
  WITH RECURSIVE inherited(role_oid) AS (
    SELECT membership.roleid FROM pg_auth_members membership
    JOIN pg_roles login ON login.oid=membership.member
    WHERE login.rolname='${runtime_user}'
    UNION
    SELECT membership.roleid FROM pg_auth_members membership
    JOIN inherited ON inherited.role_oid=membership.member
  )
  SELECT array_agg(role.rolname ORDER BY role.rolname) INTO inherited_roles
  FROM inherited JOIN pg_roles role ON role.oid=inherited.role_oid;
  IF inherited_roles<>ARRAY['agent_learning_worker']::name[] THEN
    RAISE EXCEPTION 'Draft worker role membership drift: %',inherited_roles;
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='${runtime_user}' AND (
      NOT rolcanlogin OR NOT rolinherit OR rolsuper OR rolcreatedb OR rolcreaterole
      OR rolreplication OR rolbypassrls))
     OR pg_has_role('${runtime_user}','agent_learning_owner','MEMBER')
     OR pg_has_role('${runtime_user}','agent_learning_control','MEMBER')
     OR pg_has_role('${runtime_user}','agent_cron_worker','MEMBER')
     OR has_table_privilege('${runtime_user}','agent_learning_drafts','SELECT,INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_learning_runner_attempts','SELECT,INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_learning_worker_targets','SELECT,INSERT,UPDATE,DELETE')
     OR NOT has_function_privilege('${runtime_user}',
       'agent_learning_worker_claim_checks(text,text,timestamp with time zone,integer,integer)','EXECUTE')
     OR has_function_privilege('${runtime_user}',
       'agent_learning_worker_provision_target(text,text,uuid,text,text,text,text,text,text,text,text,text,text,timestamp with time zone,timestamp with time zone,text,text)','EXECUTE')
     OR has_function_privilege('${runtime_user}',
       'agent_learning_claim_checks(text,timestamp with time zone,integer,integer)','EXECUTE')
     OR has_function_privilege('${runtime_user}',
       'agent_learning_reject(text,uuid,bigint,text,text,text,text,text)','EXECUTE')
     OR has_function_privilege('${runtime_user}',
       'agent_learning_promote(text,uuid,bigint,text,text,uuid,text,text,text,text,jsonb,text)','EXECUTE') THEN
    RAISE EXCEPTION 'Draft worker function-only authority drifted';
  END IF;
END
\$\$;" >/dev/null

log "proving direct DML, generic claims and human decisions are denied"
expect_runtime_denied direct_select "SELECT count(*) FROM agent_learning_drafts;"
expect_runtime_denied direct_update "UPDATE agent_learning_drafts SET updated_at=clock_timestamp();"
expect_runtime_denied broad_claim \
  "SELECT * FROM agent_learning_claim_checks('forged',clock_timestamp(),30,1);"
expect_runtime_denied provision \
  "SELECT agent_learning_worker_provision_target('${activation_id}','${target_draft}',
    '${target_user}'::uuid,'${target_draft_fingerprint}','${target_package_fingerprint}',
    '${target_runtime_fingerprint}','${target_archive_fingerprint}',
    '${runner_snapshot_fingerprint}','${workspace_snapshot_id}','${workspace_fingerprint}',
    '${isolation_suite_fingerprint}','${evaluation_suite_fingerprint}','${plan_fingerprint}',
    clock_timestamp(),clock_timestamp()+interval '1 hour','forged','FORGED');"
expect_runtime_denied promote \
  "SELECT * FROM agent_learning_promote('${target_draft}','${target_user}'::uuid,2,
    '${target_draft_fingerprint}','${target_package_fingerprint}',
    '00000000-0000-4000-8000-000000000001'::uuid,'draft_decision_21c5000000000001',
    'FORGED','draft:forged','skill-quarantine/forged','{}'::jsonb,
    'draft_event_21c5000000000001');"

log "claiming only the exact activation-bound Draft"
[[ "$(psql_runtime "SELECT count(*) FROM agent_learning_worker_claim_checks(
  'activation_21c500000000ffff','draft-worker-forged',clock_timestamp(),300,100);")" == "0" ]]
claim_record="$(psql_runtime "SELECT concat_ws('|',id,check_generation)
  FROM agent_learning_worker_claim_checks(
    '${activation_id}','draft-worker-exact',clock_timestamp(),300,100);")"
IFS='|' read -r claimed_draft check_generation <<<"${claim_record}"
[[ "${claimed_draft}" == "${target_draft}" ]]
[[ "${check_generation}" =~ ^[1-9][0-9]*$ ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_learning_drafts
  WHERE id='${unrelated_draft}' AND state='quarantined' AND check_owner IS NULL;")" == "1" ]]

run_runner_check() {
  local kind="$1" suffix="$2" suite_fingerprint="$3" artifact_fingerprint="$4"
  local attempt_id="attempt_21c500000000000${suffix}"
  local transport_run_id="run_21c500000000000${suffix}"
  local step_id="step_21c500000000000${suffix}"
  local sandbox_id="sandbox_21c50000000000${suffix}${suffix}"
  local request_hex response_hex spec_hex probe_hex
  request_hex="$(printf '%064d' "${suffix}" | tr ' ' '0')"
  response_hex="$(printf '%064d' "$((suffix + 2))" | tr ' ' '0')"
  spec_hex="$(printf '%064d' "$((suffix + 4))" | tr ' ' '0')"
  probe_hex="$(printf '%064d' "$((suffix + 6))" | tr ' ' '0')"
  local result_json
  result_json="{\"schemaVersion\":\"neo.agent-draft-runner-result/v1\",\"activationId\":\"${activation_id}\",\"draftId\":\"${target_draft}\",\"draftFingerprint\":\"${target_draft_fingerprint}\",\"checkGeneration\":${check_generation},\"kind\":\"${kind}\",\"proposedPackageFingerprint\":\"${target_package_fingerprint}\",\"runtimeBundleFingerprint\":\"${target_runtime_fingerprint}\",\"archiveFingerprint\":\"${target_archive_fingerprint}\",\"workspaceSnapshotId\":\"${workspace_snapshot_id}\",\"workspaceFingerprint\":\"${workspace_fingerprint}\",\"suiteFingerprint\":\"${suite_fingerprint}\",\"status\":\"passed\",\"reasonCode\":\"CHECK_PASSED\",\"durationMillis\":100,\"metrics\":{\"cases\":1}}"

  [[ "$(psql_runtime "SELECT created FROM agent_learning_worker_begin_runner_check(
    '${activation_id}','${target_draft}','draft-worker-exact',${check_generation},
    '${kind}','${attempt_id}','${transport_run_id}','${step_id}',1,'${runner_id}',
    '${lease_token_hash}','${runner_snapshot_fingerprint}',clock_timestamp(),300);")" == "t" ]]
  [[ "$(psql_runtime "SELECT replayed FROM agent_learning_worker_issue_runner_authority(
    '${activation_id}','spiffe://neo-chat/agent-runtime-draft-learning','${runner_id}',
    'rpc_21c500000000010${suffix}','nonce21c500000010${suffix}','launch',
    'sha256:${request_hex}','${attempt_id}','${lease_token_hash}',10);")" == "f" ]]
  [[ "$(psql_runtime "SELECT agent_learning_worker_complete_runner_request(
    'spiffe://neo-chat/agent-runtime-draft-learning','rpc_21c500000000010${suffix}',
    'nonce21c500000010${suffix}','sha256:${request_hex}','sha256:${response_hex}',
    '{\"status\":\"accepted\"}'::jsonb);")" == "t" ]]
  [[ "$(psql_runtime "SELECT replayed FROM agent_learning_worker_issue_runner_authority(
    '${activation_id}','spiffe://neo-chat/agent-runtime-draft-learning','${runner_id}',
    'rpc_21c500000000010${suffix}','nonce21c500000010${suffix}','launch',
    'sha256:${request_hex}','${attempt_id}','${lease_token_hash}',10);")" == "t" ]]
  [[ "$(psql_runtime "SELECT agent_learning_worker_record_runner_launch(
    '${activation_id}','${attempt_id}',1,'${sandbox_id}',
    'sha256:${spec_hex}','sha256:${probe_hex}');")" == "t" ]]

  [[ "$(psql_runtime "SELECT replayed FROM agent_learning_worker_issue_runner_authority(
    '${activation_id}','spiffe://neo-chat/agent-runtime-draft-learning','${runner_id}',
    'rpc_21c500000000020${suffix}','nonce21c500000020${suffix}','result',
    'sha256:${response_hex}','${attempt_id}','${lease_token_hash}',10);")" == "f" ]]
  [[ "$(psql_runtime "SELECT agent_learning_worker_complete_runner_request(
    'spiffe://neo-chat/agent-runtime-draft-learning','rpc_21c500000000020${suffix}',
    'nonce21c500000020${suffix}','sha256:${response_hex}','sha256:${spec_hex}',
    '{\"status\":\"ready\"}'::jsonb);")" == "t" ]]
  [[ "$(psql_runtime "SELECT agent_learning_worker_record_runner_result(
    '${activation_id}','${attempt_id}',1,'draft-check-result.json',512,
    '${artifact_fingerprint}','${result_json}'::jsonb);")" == "t" ]]
  [[ "$(psql_runtime "SELECT agent_learning_worker_record_runner_result(
    '${activation_id}','${attempt_id}',1,'draft-check-result.json',512,
    '${artifact_fingerprint}','${result_json}'::jsonb);")" == "f" ]]

  [[ "$(psql_runtime "SELECT agent_learning_worker_mark_runner_cancel_pending(
    '${activation_id}','${attempt_id}',1,'RUNNER_CANCEL_REQUESTED');")" == "t" ]]
  [[ "$(psql_runtime "SELECT replayed FROM agent_learning_worker_issue_runner_authority(
    '${activation_id}','spiffe://neo-chat/agent-runtime-draft-learning','${runner_id}',
    'rpc_21c500000000030${suffix}','nonce21c500000030${suffix}','cancel',
    'sha256:${probe_hex}','${attempt_id}','${lease_token_hash}',10);")" == "f" ]]
  [[ "$(psql_runtime "SELECT agent_learning_worker_complete_runner_request(
    'spiffe://neo-chat/agent-runtime-draft-learning','rpc_21c500000000030${suffix}',
    'nonce21c500000030${suffix}','sha256:${probe_hex}','sha256:${request_hex}',
    '{\"status\":\"canceled\"}'::jsonb);")" == "t" ]]
  [[ "$(psql_runtime "SELECT agent_learning_worker_complete_runner_cleanup(
    '${activation_id}','${attempt_id}',1,true,'');")" == "t" ]]
  [[ "$(psql_runtime "SELECT agent_learning_worker_complete_runner_cleanup(
    '${activation_id}','${attempt_id}',1,true,'');")" == "f" ]]
}

log "proving isolation and evaluation launch/result/cancel replay chains"
isolation_artifact_fingerprint="sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
evaluation_artifact_fingerprint="sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
run_runner_check isolation 1 "${isolation_suite_fingerprint}" "${isolation_artifact_fingerprint}"
run_runner_check evaluation 2 "${evaluation_suite_fingerprint}" "${evaluation_artifact_fingerprint}"
[[ "$(psql_runtime "SELECT count(*) FROM agent_learning_worker_runner_inventory(
  '${activation_id}',100);")" == "0" ]]

log "persisting the exact three-receipt bundle and preserving human Promote separation"
receipt_json="[{\"id\":\"draft_check_21c5000000000001\",\"kind\":\"static\",\"status\":\"passed\",\"reasonCode\":\"CHECK_PASSED\",\"suiteFingerprint\":\"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\",\"evidenceFingerprint\":\"sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\",\"durationMillis\":10,\"metrics\":{\"cases\":1}},{\"id\":\"draft_check_21c5000000000002\",\"kind\":\"isolation\",\"status\":\"passed\",\"reasonCode\":\"CHECK_PASSED\",\"suiteFingerprint\":\"${isolation_suite_fingerprint}\",\"evidenceFingerprint\":\"${isolation_artifact_fingerprint}\",\"durationMillis\":100,\"metrics\":{\"cases\":1}},{\"id\":\"draft_check_21c5000000000003\",\"kind\":\"evaluation\",\"status\":\"passed\",\"reasonCode\":\"CHECK_PASSED\",\"suiteFingerprint\":\"${evaluation_suite_fingerprint}\",\"evidenceFingerprint\":\"${evaluation_artifact_fingerprint}\",\"durationMillis\":100,\"metrics\":{\"cases\":1}}]"
[[ "$(psql_runtime "SELECT state FROM agent_learning_worker_complete_checks(
  '${activation_id}','${target_draft}','draft-worker-exact',${check_generation},
  '${receipt_json}'::jsonb,'draft_event_21c5000000001001');")" == "reviewable" ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_learning_check_results
  WHERE draft_id='${target_draft}' AND generation=${check_generation} AND status='passed';")" == "3" ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM skill_package_candidates
  WHERE source_type='learning' AND package_fingerprint='${target_package_fingerprint}';")" == "0" ]]

log "creating a separate human Promote fact and completing target-scoped cleanup"
promotion_record="$(psql_command "${container_name}" "
SELECT concat_ws('|',draft_id,admission_id,package_fingerprint,created)
FROM agent_learning_promote(
  '${target_draft}','${target_user}'::uuid,2,'${target_draft_fingerprint}',
  '${target_package_fingerprint}','21c50000-0000-4000-8000-000000001001'::uuid,
  'draft_decision_21c5000000001001','G21_5_DRAFT_PROMOTED',
  'draft:${target_draft}:${target_draft_fingerprint}',
  'skill-quarantine/sha256/${target_archive_fingerprint#sha256:}.zip',
  jsonb_build_object(
    'runtimeBundleFingerprint','${target_runtime_fingerprint}',
    'sbomFingerprint','${target_sbom_fingerprint}',
    'name','${target_name}','version','${target_version}',
    'description','G21.5 retained Draft','license','MIT','compatibility','',
    'allowedTools',jsonb_build_array('Read','Search'),
    'capabilityRequests',jsonb_build_array(jsonb_build_object(
      'capability','workspace.read','actions',jsonb_build_array('list','read'),
      'reason','Read only learning fixture.')),
    'hasRuntime',true,'fileCount',3,'packageBytes',${target_archive_bytes},
    'expandedBytes',1048576,
    'packageObjectKey','skill-packages/sha256/${target_package_fingerprint#sha256:}.zip',
    'sbomObjectKey','skill-sboms/sha256/${target_sbom_fingerprint#sha256:}.cdx.json'),
  'draft_event_21c5000000001002');")"
IFS='|' read -r promoted_draft _ promoted_fingerprint promotion_created <<<"${promotion_record}"
[[ "${promoted_draft}" == "${target_draft}" ]]
[[ "${promoted_fingerprint}" == "${target_package_fingerprint}" ]]
[[ "${promotion_created}" == "t" ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM skill_package_candidates
  WHERE source_type='learning' AND package_fingerprint='${target_package_fingerprint}'
    AND status='admitted';")" == "1" ]]
cleanup_record="$(psql_runtime "SELECT concat_ws('|',draft_id,generation,object_fingerprint)
  FROM agent_learning_worker_claim_cleanup(
    '${activation_id}','draft-cleaner-exact',clock_timestamp()+interval '8 days',300,100);")"
IFS='|' read -r cleanup_draft cleanup_generation cleanup_fingerprint <<<"${cleanup_record}"
[[ "${cleanup_draft}" == "${target_draft}" ]]
[[ "${cleanup_fingerprint}" == "${target_archive_fingerprint}" ]]
psql_runtime "SELECT agent_learning_worker_complete_cleanup(
  '${activation_id}','${target_draft}','draft-cleaner-exact',${cleanup_generation},
  '${target_archive_fingerprint}');" >/dev/null
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_learning_cleanup_queue
  WHERE draft_id='${target_draft}';")" == "0" ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_learning_drafts
  WHERE id='${unrelated_draft}' AND state='quarantined' AND object_deleted_at IS NULL;")" == "1" ]]
[[ "$(psql_runtime "SELECT checks_reclaimed+cleanup_reclaimed
  FROM agent_learning_worker_reconcile('${activation_id}',clock_timestamp(),100);")" == "0" ]]

log "dumping and restoring Draft target, result, receipt and cleanup authority"
docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
  pg_dump --no-owner --no-privileges -U "${database_user}" -d "${database_name}" \
  >"${work_dir}/authority.sql"
source_counts="$(psql_command "${container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_learning_worker_targets),
  (SELECT count(*) FROM agent_learning_runner_attempts),
  (SELECT count(*) FROM agent_learning_runner_results),
  (SELECT count(*) FROM agent_learning_runner_requests),
  (SELECT count(*) FROM agent_learning_check_results),
  (SELECT count(*) FROM agent_learning_cleanup_queue));")"
start_database "${restore_container_name}"
docker exec -i -e "PGPASSWORD=${database_password}" "${restore_container_name}" \
  psql --set=ON_ERROR_STOP=1 --no-psqlrc -U "${database_user}" -d "${database_name}" \
  <"${work_dir}/authority.sql" >/dev/null
restore_counts="$(psql_command "${restore_container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_learning_worker_targets),
  (SELECT count(*) FROM agent_learning_runner_attempts),
  (SELECT count(*) FROM agent_learning_runner_results),
  (SELECT count(*) FROM agent_learning_runner_requests),
  (SELECT count(*) FROM agent_learning_check_results),
  (SELECT count(*) FROM agent_learning_cleanup_queue));")"
[[ "${source_counts}" == "${restore_counts}" ]]

run_migrate down >"${work_dir}/peel-097-chat-agent-goal-tail.log" 2>&1
grep -Fq "down 097_chat_agent_goals" "${work_dir}/peel-097-chat-agent-goal-tail.log"
run_migrate down >"${work_dir}/peel-096-chat-agent-event-tail.log" 2>&1
grep -Fq "down 096_chat_agent_event_log" "${work_dir}/peel-096-chat-agent-event-tail.log"
run_migrate down >"${work_dir}/peel-095-tail.log" 2>&1
grep -Fq "down 095_agent_product_canary_activation" "${work_dir}/peel-095-tail.log"
log "proving active, LOGIN-membership and retained-fact down guards"
set +e
run_migrate down >"${work_dir}/active-guard.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] ||
   ! grep -Fq 'AGENT_WORKER_DOWN_ACTIVE_TARGET' "${work_dir}/active-guard.log"; then
  cat "${work_dir}/active-guard.log" >&2
  exit 1
fi
psql_command "${container_name}" "SELECT agent_learning_worker_disable_target(
  '${activation_id}','g21.5-draft-operator','G21_5_DRAFT_DISABLED');" >/dev/null
set +e
run_migrate down >"${work_dir}/membership-guard.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] ||
   ! grep -Fq 'AGENT_WORKER_DOWN_LOGIN_MEMBERSHIP' "${work_dir}/membership-guard.log"; then
  cat "${work_dir}/membership-guard.log" >&2
  exit 1
fi
psql_command "${container_name}" "REVOKE agent_learning_worker FROM ${runtime_user}; DROP ROLE ${runtime_user};" >/dev/null
set +e
run_migrate down >"${work_dir}/retained-guard.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] ||
   ! grep -Fq 'AGENT_WORKER_DOWN_RETAINED_ACTIVATION_FACTS' "${work_dir}/retained-guard.log"; then
  cat "${work_dir}/retained-guard.log" >&2
  exit 1
fi

log "proving clean disposable 094 down/up and final head 097"
start_database "${clean_container_name}"
clean_database_url="$(database_url_for "${clean_container_name}")"
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" up \
  >"${work_dir}/clean-up.log" 2>&1
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" down \
  >"${work_dir}/clean-down-097.log" 2>&1
grep -Fq "down 097_chat_agent_goals" "${work_dir}/clean-down-097.log"
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" down \
  >"${work_dir}/clean-down-096.log" 2>&1
grep -Fq "down 096_chat_agent_event_log" "${work_dir}/clean-down-096.log"
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" down \
  >"${work_dir}/clean-down-095.log" 2>&1
grep -Fq "down 095_agent_product_canary_activation" "${work_dir}/clean-down-095.log"
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" down \
  >"${work_dir}/clean-down.log" 2>&1
grep -Fq "down 094_agent_cron_learning_activation" "${work_dir}/clean-down.log"
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" up \
  >"${work_dir}/clean-reup.log" 2>&1
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/clean-reup.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/clean-reup.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/clean-reup.log"
grep -Fq "up 097_chat_agent_goals" "${work_dir}/clean-reup.log"
[[ "$(psql_command "${clean_container_name}" 'SELECT max(version) FROM schema_migrations')" == "97" ]]

log "passed (fresh/replay, exact LOGIN, scoped Draft claim, Runner result replay, human decision separation, cleanup, dump/restore, guarded and clean down/up)"
