#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-product-pg17-$RANDOM-$$"
restore_container_name="${container_name}-restore"
database_name="neo_chat_agent_product_drill"
database_user="postgres"
database_password="agent-product-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker stop "${container_name}" "${restore_container_name}" >/dev/null 2>&1 || true
  find "${work_dir}" -depth -mindepth 1 -delete
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
log() { printf 'Agent product/Shadow PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker openssl; do
  command -v "${command}" >/dev/null 2>&1 || { echo "${command} is required" >&2; exit 1; }
done
docker version >/dev/null 2>&1 || { echo "Docker is required" >&2; exit 1; }
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
    docker exec "${name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null 2>&1 && return
    sleep 1
  done
  exit 1
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

log "starting disposable database and applying 001 -> 092"
start_database "${container_name}"
database_url="$(database_url_for "${container_name}")"
[[ "$(psql_command "${container_name}" 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" "$@"; }
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 090_agent_product_shadow" "${work_dir}/fresh.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/fresh.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/fresh.log"
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"

log "checking least privilege and bounded product projections"
psql_command "${container_name}" "
DO \$\$
BEGIN
  IF NOT has_table_privilege('go_api_runtime','agent_product_runs','SELECT')
     OR has_table_privilege('go_api_runtime','agent_runs','SELECT')
     OR has_table_privilege('go_api_runtime','agent_product_run_cancellations','SELECT')
     OR has_table_privilege('go_api_runtime','agent_product_run_cancellations','INSERT')
     OR has_table_privilege('go_api_runtime','agent_artifacts','SELECT')
     OR has_table_privilege('go_api_runtime','agent_shadow_observations','INSERT')
     OR NOT has_function_privilege('go_api_runtime','agent_product_get_artifact(uuid,text,text)','EXECUTE')
     OR NOT has_function_privilege('go_api_runtime','agent_product_cancel_run(text,uuid,text,text,text,text,text)','EXECUTE')
     OR NOT has_function_privilege('go_api_runtime','agent_product_shadow_snapshot(uuid)','EXECUTE')
     OR has_function_privilege('go_api_runtime','agent_orchestrator_active_kill_mode(text[])','EXECUTE')
     OR NOT has_function_privilege('agent_product_owner','agent_orchestrator_active_kill_mode(text[])','EXECUTE')
     OR has_function_privilege('agent_runner_control','agent_product_append_shadow_observation(text,uuid,bigint,bigint,uuid,text,text,text,text,text,text,integer,integer,integer,integer)','EXECUTE')
     OR pg_has_role('go_api_runtime','agent_product_owner','MEMBER')
     OR (SELECT rolcanlogin OR rolsuper OR rolcreaterole OR rolbypassrls FROM pg_roles WHERE rolname='agent_product_owner') THEN
    RAISE EXCEPTION 'Agent product grants violate least privilege';
  END IF;
END
\$\$;" >/dev/null

user_one="11111111-1111-4111-8111-111111111111"
user_two="22222222-2222-4222-8222-222222222222"
admission="33333333-3333-4333-8333-333333333333"
fp_a="sha256:$(printf 'a%.0s' {1..64})"
fp_b="sha256:$(printf 'b%.0s' {1..64})"
fp_c="sha256:$(printf 'c%.0s' {1..64})"
fp_d="sha256:$(printf 'd%.0s' {1..64})"

log "seeding sanitized authority fixtures"
psql_command "${container_name}" "
INSERT INTO users(id,display_name) VALUES('${user_one}','one'),('${user_two}','two');
INSERT INTO skill_package_versions(package_fingerprint,runtime_bundle_fingerprint,sbom_fingerprint,
  name,version,description,allowed_tools,capability_requests,has_runtime,file_count,
  package_bytes,expanded_bytes,package_object_key,sbom_object_key)
VALUES('${fp_a}','${fp_b}','${fp_c}','shadow-probe','1.0.0','synthetic fixture','[]','[]',true,1,1,1,
  'skill-packages/sha256/'||substring('${fp_a}' FROM 8)||'.zip',
  'skill-sboms/sha256/'||substring('${fp_c}' FROM 8)||'.cdx.json');
INSERT INTO skill_package_candidates(id,source_type,source_ref,source_artifact_sha256,
  source_object_key,package_fingerprint,status,admission_eligible,reviewed_by_user_id)
VALUES('${admission}','official','official:shadow-probe@1.0.0','${fp_d}',
  'skill-quarantine/sha256/'||substring('${fp_d}' FROM 8)||'.zip','${fp_a}','admitted',true,'${user_one}');

INSERT INTO agent_run_snapshots(id,user_id,fingerprint,canonical_snapshot)
VALUES('snapshot_cancel1234567890','${user_one}','${fp_a}','{}'),
      ('snapshot_artifact12345678','${user_one}','${fp_b}','{}');
INSERT INTO agent_runs(id,user_id,snapshot_id,snapshot_fingerprint,idempotency_key,
  request_fingerprint,state,scope_keys,created_at,updated_at,terminal_at)
VALUES('run_cancel1234567890','${user_one}','snapshot_cancel1234567890','${fp_a}','cancel-fixture','${fp_c}',
  'pending',ARRAY['global:*','scheduler:*','user:${user_one}','run:run_cancel1234567890'],
  TIMESTAMPTZ '2026-01-01 00:00:00+00',TIMESTAMPTZ '2026-01-01 00:00:00+00',NULL),
 ('run_artifact12345678','${user_one}','snapshot_artifact12345678','${fp_b}','artifact-fixture','${fp_d}',
  'succeeded',ARRAY['global:*','scheduler:*','user:${user_one}','run:run_artifact12345678'],
  TIMESTAMPTZ '2026-01-01 00:00:00+00',TIMESTAMPTZ '2026-01-01 00:00:01+00',
  TIMESTAMPTZ '2026-01-01 00:00:01+00');
INSERT INTO agent_steps(id,run_id,user_id,ordinal,kind,state,current_generation,created_at,updated_at,terminal_at)
VALUES('step_artifact12345678','run_artifact12345678','${user_one}',0,'inspect','succeeded',1,
  TIMESTAMPTZ '2026-01-01 00:00:00+00',TIMESTAMPTZ '2026-01-01 00:00:01+00',
  TIMESTAMPTZ '2026-01-01 00:00:01+00');
INSERT INTO agent_attempts(id,run_id,user_id,step_id,generation,state,lease_owner,lease_token_hash,
  lease_expires_at,created_at,updated_at,terminal_at)
VALUES('attempt_artifact12345678','run_artifact12345678','${user_one}','step_artifact12345678',1,'succeeded',
  'runner-test','$(printf '1%.0s' {1..64})',TIMESTAMPTZ '2026-01-01 00:01:00+00',
  TIMESTAMPTZ '2026-01-01 00:00:00+00',TIMESTAMPTZ '2026-01-01 00:00:01+00',
  TIMESTAMPTZ '2026-01-01 00:00:01+00');
INSERT INTO agent_artifacts(id,run_id,user_id,attempt_id,generation,name,media_type,size_bytes,fingerprint,object_key)
VALUES('artifact_report1234567890','run_artifact12345678','${user_one}','attempt_artifact12345678',1,
  'report.txt','text/plain',6,'${fp_a}',
  'agent-artifacts/run_artifact12345678/attempt_artifact12345678/1/'||substring('${fp_a}' FROM 8));
" >/dev/null

log "proving user binding, artifact seam, cancellation replay, and stale denial"
projection_counts="$(psql_command "${container_name}" "SET ROLE go_api_runtime;
SELECT concat_ws(',',
  (SELECT count(*) FROM agent_product_runs WHERE user_id='${user_one}'),
  (SELECT count(*) FROM agent_product_runs WHERE user_id='${user_two}'),
  (SELECT count(*) FROM agent_product_get_artifact(
    '${user_one}','run_artifact12345678','artifact_report1234567890') WHERE object_key LIKE 'agent-artifacts/%'));" | tail -n 1)"
[[ "${projection_counts}" == "2,0,1" ]]
psql_command "${container_name}" "SET ROLE go_api_runtime;
SELECT state FROM agent_product_cancel_run('cancellation_product1234567890','${user_one}',
  'run_cancel1234567890','pending','${fp_a}','cancel','USER_REQUESTED');
SELECT state FROM agent_product_cancel_run('cancellation_product1234567890','${user_one}',
  'run_cancel1234567890','pending','${fp_a}','cancel','USER_REQUESTED');" >"${work_dir}/product.log"
grep -Fq "completed" "${work_dir}/product.log"
set +e
psql_command "${container_name}" "SET ROLE go_api_runtime; SELECT * FROM agent_product_cancel_run(
  'cancellation_crossuser1234567890','${user_two}','run_artifact12345678','succeeded','${fp_b}','cancel','USER_REQUESTED');" \
  >"${work_dir}/cross-user.log" 2>&1
cross_status=$?
set -e
[[ "${cross_status}" -ne 0 ]]
grep -Fq "AGENT_PRODUCT_RUN_NOT_FOUND" "${work_dir}/cross-user.log"

log "proving default-off Shadow, cohort, budget, restart, generation, and fingerprint fences"
snapshot_before="$(psql_command "${container_name}" "SET ROLE go_api_runtime; SELECT held_reason_code FROM agent_product_shadow_snapshot('${user_one}');")"
[[ "${snapshot_before}" == *"SHADOW_DISABLED"* ]]
psql_command "${container_name}" "SET ROLE go_api_runtime;
SELECT revision FROM agent_product_update_shadow_policy('${user_one}',0,true,'synthetic','${admission}',
  '${fp_a}','${fp_b}',10000,1,0,clock_timestamp()-interval '1 minute',clock_timestamp()+interval '1 hour','ADMIN_ENABLE');
SELECT agent_product_register_shadow_boot('boot_productshadow1234');
SELECT generation FROM agent_product_set_shadow_opt_in('${user_one}',0,1,true,'USER_OPT_IN');
SELECT concat_ws(',',eligible,effective,held_reason_code)
  FROM agent_product_shadow_snapshot('${user_one}');" \
  >"${work_dir}/shadow.log"
grep -Fxq "t,f,ISOLATION_UNAVAILABLE" "${work_dir}/shadow.log"

assert_rejected() {
  local name="$1" expected="$2" sql="$3" status
  set +e
  psql_command "${container_name}" "SET ROLE go_api_runtime; ${sql}" >"${work_dir}/${name}.log" 2>&1
  status=$?
  set -e
  [[ "${status}" -ne 0 ]] || { cat "${work_dir}/${name}.log" >&2; exit 1; }
  grep -Fq "${expected}" "${work_dir}/${name}.log"
}
psql_command "${container_name}" "SELECT * FROM agent_orchestrator_append_kill_switch(
  'switch_shadowkill123456','global','*','deny_new',true,0,'operator','product-shadow-drill','SHADOW_HOLD');" >/dev/null
kill_snapshot="$(psql_command "${container_name}" "SET ROLE go_api_runtime;
  SELECT held_reason_code FROM agent_product_shadow_snapshot('${user_one}');")"
[[ "${kill_snapshot}" == *"KILL_SWITCH_ACTIVE"* ]]
assert_rejected kill_switch KILL_SWITCH_ACTIVE "SELECT * FROM agent_product_append_shadow_observation(
  'boot_productshadow1234','${user_one}',1,1,'${admission}','${fp_a}','${fp_b}',
  'synthetic','held','KILL_SWITCH_ACTIVE','lt_100ms',0,0,0,0);"
psql_command "${container_name}" "SELECT * FROM agent_orchestrator_append_kill_switch(
  'switch_shadowkill123456','global','*','deny_new',false,1,'operator','product-shadow-drill','SHADOW_RESUME');" >/dev/null
psql_command "${container_name}" "SET ROLE go_api_runtime;
SELECT observation_id FROM agent_product_append_shadow_observation('boot_productshadow1234','${user_one}',1,1,
  '${admission}','${fp_a}','${fp_b}','synthetic','succeeded','SYNTHETIC_OK','lt_100ms',1,1,1,0);" >/dev/null
assert_rejected budget BUDGET_EXCEEDED "SELECT * FROM agent_product_append_shadow_observation(
  'boot_productshadow1234','${user_one}',1,1,'${admission}','${fp_a}','${fp_b}',
  'synthetic','succeeded','SYNTHETIC_OK','lt_100ms',1,1,1,0);"
assert_rejected stale_opt GENERATION_STALE "SELECT * FROM agent_product_set_shadow_opt_in(
  '${user_one}',0,1,false,'USER_OPT_OUT');"
assert_rejected stale_policy REVISION_CONFLICT "SELECT * FROM agent_product_update_shadow_policy(
  '${user_one}',0,false,'synthetic',NULL,'','',0,1,0,NULL,NULL,'ADMIN_DISABLE');"
assert_rejected fingerprint GENERATION_STALE "SELECT * FROM agent_product_append_shadow_observation(
  'boot_productshadow1234','${user_one}',1,1,'${admission}','${fp_c}','${fp_b}',
  'synthetic','held','FINGERPRINT_DRIFT','lt_100ms',0,0,0,0);"
psql_command "${container_name}" "SET ROLE go_api_runtime; SELECT agent_product_register_shadow_boot('boot_productshadow5678');" >/dev/null
assert_rejected restart GENERATION_STALE "SELECT * FROM agent_product_append_shadow_observation(
  'boot_productshadow1234','${user_one}',1,1,'${admission}','${fp_a}','${fp_b}',
  'synthetic','held','RESTART_FENCE','lt_100ms',0,0,0,0);"
[[ "$(psql_command "${container_name}" 'SELECT count(*) FROM agent_shadow_observations')" == "1" ]]

log "checking content-free schema and dump/restore authority"
psql_command "${container_name}" "
DO \$\$
BEGIN
  IF EXISTS(SELECT 1 FROM information_schema.columns
    WHERE table_schema=current_schema() AND table_name IN ('agent_shadow_observations','agent_product_run_cancellations')
      AND lower(column_name) ~ '(prompt|body|argument|secret|output|object_key)') THEN
    RAISE EXCEPTION 'content-bearing diagnostic column found';
  END IF;
END
\$\$;" >/dev/null
docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
  pg_dump --no-owner --no-privileges -U "${database_user}" -d "${database_name}" >"${work_dir}/authority.sql"
source_counts="$(psql_command "${container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_product_run_cancellations),(SELECT count(*) FROM agent_artifacts),
  (SELECT count(*) FROM agent_shadow_policies),(SELECT count(*) FROM agent_shadow_user_opt_ins),
  (SELECT count(*) FROM agent_shadow_observations));")"
start_database "${restore_container_name}"
docker exec -i -e "PGPASSWORD=${database_password}" "${restore_container_name}" \
  psql --set=ON_ERROR_STOP=1 --no-psqlrc -U "${database_user}" -d "${database_name}" \
  <"${work_dir}/authority.sql" >/dev/null
restore_counts="$(psql_command "${restore_container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_product_run_cancellations),(SELECT count(*) FROM agent_artifacts),
  (SELECT count(*) FROM agent_shadow_policies),(SELECT count(*) FROM agent_shadow_user_opt_ins),
  (SELECT count(*) FROM agent_shadow_observations));")"
[[ "${source_counts}" == "${restore_counts}" ]]

log "proving guarded down and clean 089 -> 092 -> 089 -> 092"
run_migrate down >"${work_dir}/peel-092.log" 2>&1
grep -Fq "down 092_agent_project_mutation_canary" "${work_dir}/peel-092.log"
run_migrate down >"${work_dir}/peel-091.log" 2>&1
grep -Fq "down 091_agent_artifact_publication" "${work_dir}/peel-091.log"
set +e
run_migrate down >"${work_dir}/guard.log" 2>&1
guard_status=$?
set -e
[[ "${guard_status}" -ne 0 ]]
grep -Fq "AGENT_PRODUCT_DOWN_DATA_EXISTS" "${work_dir}/guard.log"
psql_command "${container_name}" "TRUNCATE TABLE agent_shadow_observations,agent_shadow_user_opt_ins,
  agent_shadow_policies,agent_artifacts,agent_product_run_cancellations CASCADE;" >/dev/null
run_migrate down >"${work_dir}/down.log" 2>&1
grep -Fq "down 090_agent_product_shadow" "${work_dir}/down.log"
run_migrate up >"${work_dir}/reup.log" 2>&1
grep -Fq "up 090_agent_product_shadow" "${work_dir}/reup.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/reup.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/reup.log"
run_migrate up >"${work_dir}/final.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/final.log"
log "passed (fresh/replay, ACLs, ownership, Artifact/cancel, Shadow fences/budget/restart, content-free dump/restore, guarded down/up)"
