#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-product-canary-pg17-$RANDOM-$$"
restore_container_name="${container_name}-restore"
database_name="neo_chat_agent_product_canary_drill"
database_user="postgres"
database_password="agent-product-canary-drill-$(openssl rand -hex 16)"
runtime_user="agent_product_canary_app"
runtime_password="product-canary-runtime-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker stop "${container_name}" "${restore_container_name}" >/dev/null 2>&1 || true
  find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
log() { printf 'Agent product canary PostgreSQL 17 drill: %s\n' "$*"; }
canonical_sha256() {
  local domain="$1" digest
  shift
  digest="$({
    printf '%s' "${domain}"
    for component in "$@"; do
      printf '\0%s' "${component}"
    done
  } | sha256sum)"
  printf 'sha256:%s' "${digest%% *}"
}

for command in docker go openssl sha256sum; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Agent product canary PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Agent product canary PostgreSQL 17 drill: Docker is required" >&2
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
assert_rejected() {
  local role="$1" name="$2" expected="$3" sql="$4" status
  set +e
  psql_command "${container_name}" "SET ROLE ${role}; ${sql}" >"${work_dir}/${name}.log" 2>&1
  status=$?
  set -e
  if [[ "${status}" -eq 0 ]]; then
    cat "${work_dir}/${name}.log" >&2
    exit 1
  fi
  grep -Fq "${expected}" "${work_dir}/${name}.log"
}

log "starting disposable database and applying 001 -> 096"
start_database "${container_name}"
database_url="$(database_url_for "${container_name}")"
[[ "$(psql_command "${container_name}" 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -buildvcs=false -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" "$@"; }
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 090_agent_product_shadow" "${work_dir}/fresh.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/fresh.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/fresh.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/fresh.log"
[[ "$(psql_command "${container_name}" 'SELECT max(version) FROM schema_migrations')" == "96" ]]
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"

log "provisioning the thirteenth exact-membership LOGIN and proving ACL separation"
psql_command "${container_name}" "
CREATE ROLE ${runtime_user} LOGIN INHERIT PASSWORD '${runtime_password}';
GRANT agent_product_canary_worker,agent_orchestrator_runtime,agent_runner_control TO ${runtime_user};
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
  IF inherited_roles <> ARRAY['agent_orchestrator_runtime','agent_product_canary_worker',
    'agent_runner_control']::name[] THEN
    RAISE EXCEPTION 'Product canary LOGIN membership drift: %',inherited_roles;
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='${runtime_user}' AND
    (NOT rolcanlogin OR NOT rolinherit OR rolsuper OR rolcreatedb OR rolcreaterole
      OR rolreplication OR rolbypassrls)) THEN
    RAISE EXCEPTION 'Product canary LOGIN attributes drifted';
  END IF;
  IF has_table_privilege('${runtime_user}','agent_product_canary_requests','SELECT,INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_product_canary_receipts','SELECT,INSERT,UPDATE,DELETE')
     OR has_table_privilege('go_api_runtime','agent_product_canary_requests','SELECT,INSERT,UPDATE,DELETE')
     OR NOT has_function_privilege('go_api_runtime',
       'agent_product_canary_status(uuid)','EXECUTE')
     OR NOT has_function_privilege('go_api_runtime',
       'agent_product_canary_enqueue(text,uuid,bigint,bigint,text)','EXECUTE')
     OR has_function_privilege('go_api_runtime',
       'agent_product_canary_worker_claim_requests(text,text,timestamp with time zone,integer,integer)','EXECUTE')
     OR has_function_privilege('${runtime_user}',
       'agent_product_canary_provision_activation(text,text,bigint,uuid,text,text,text,text,text,text,text,text,text,text,integer,timestamp with time zone,timestamp with time zone,text,text)','EXECUTE')
     OR has_function_privilege('${runtime_user}',
       'agent_product_canary_record_promotion(text,text,text,text,text,text,text,text,text)','EXECUTE')
     OR NOT has_function_privilege('${runtime_user}',
       'agent_product_canary_worker_claim_requests(text,text,timestamp with time zone,integer,integer)','EXECUTE')
     OR NOT has_function_privilege('${runtime_user}',
       'agent_product_canary_worker_health(text,timestamp with time zone)','EXECUTE')
     OR NOT has_function_privilege('agent_product_canary_worker',
       'agent_product_canary_worker_health(text,timestamp with time zone)','EXECUTE')
     OR has_function_privilege('go_api_runtime',
       'agent_product_canary_worker_health(text,timestamp with time zone)','EXECUTE')
     OR pg_has_role('go_api_runtime','agent_product_canary_worker','MEMBER')
     OR pg_has_role('agent_product_canary_worker','agent_product_owner','MEMBER') THEN
    RAISE EXCEPTION 'Product canary least privilege drifted';
  END IF;
END
\$\$;" >/dev/null

user_one="11111111-1111-4111-8111-111111111111"
user_two="22222222-2222-4222-8222-222222222222"
admission="33333333-3333-4333-8333-333333333333"
activation="activation_1234567890abcdef"
request_candidate_one="product_request_1234567890abcdef"
request_candidate_two="product_request_replay1234567890"
request_one="${request_candidate_one}"
run_id="run_product1234567890"
step_id="step_product123456789"
attempt_id="attempt_product123456789"
release_commit="$(printf 'a%.0s' {1..40})"
fp_package="sha256:$(printf 'a%.0s' {1..64})"
fp_runtime="sha256:$(printf 'b%.0s' {1..64})"
fp_sbom="sha256:$(printf 'c%.0s' {1..64})"
fp_source="sha256:$(printf 'd%.0s' {1..64})"
fp_plan="sha256:$(printf 'e%.0s' {1..64})"
fp_snapshot="sha256:$(printf 'f%.0s' {1..64})"
fp_request="$(canonical_sha256 'neo.agent-product-canary-request/v1' "${user_one}" 1 1)"
fp_request_two="$(canonical_sha256 'neo.agent-product-canary-request/v1' "${user_two}" 1 1)"
fp_closure="sha256:$(printf 'c%.0s' {1..64})"
fp_control="sha256:$(printf '1%.0s' {1..64})"
fp_root="sha256:$(printf '2%.0s' {1..64})"
fp_broker="sha256:$(printf '3%.0s' {1..64})"
fp_project="sha256:$(printf '4%.0s' {1..64})"
fp_child="sha256:$(printf '5%.0s' {1..64})"
fp_cron="sha256:$(printf '6%.0s' {1..64})"
fp_learning="sha256:$(printf '7%.0s' {1..64})"

log "seeding admitted Runtime and deterministic opt-in authority"
psql_command "${container_name}" "
INSERT INTO users(id,display_name) VALUES('${user_one}','one'),('${user_two}','two');
INSERT INTO skill_package_versions(package_fingerprint,runtime_bundle_fingerprint,sbom_fingerprint,
  name,version,description,allowed_tools,capability_requests,has_runtime,file_count,
  package_bytes,expanded_bytes,package_object_key,sbom_object_key)
VALUES('${fp_package}','${fp_runtime}','${fp_sbom}','product-canary','1.0.0','synthetic fixture',
  '[]','[]',true,1,1,1,
  'skill-packages/sha256/'||substring('${fp_package}' FROM 8)||'.zip',
  'skill-sboms/sha256/'||substring('${fp_sbom}' FROM 8)||'.cdx.json');
INSERT INTO skill_package_candidates(id,source_type,source_ref,source_artifact_sha256,
  source_object_key,package_fingerprint,status,admission_eligible,reviewed_by_user_id)
VALUES('${admission}','official','official:product-canary@1.0.0','${fp_source}',
  'skill-quarantine/sha256/'||substring('${fp_source}' FROM 8)||'.zip',
  '${fp_package}','admitted',true,'${user_one}');
SET ROLE go_api_runtime;
SELECT revision FROM agent_product_update_shadow_policy('${user_one}',0,true,'read_only','${admission}',
  '${fp_package}','${fp_runtime}',10000,20,2,clock_timestamp()-interval '5 minutes',
  clock_timestamp()+interval '2 hours','PRODUCT_CANARY_ENABLE');
SELECT generation FROM agent_product_set_shadow_opt_in('${user_one}',0,1,true,'USER_OPT_IN');" \
  >"${work_dir}/shadow-authority.log"
grep -Fxq "1" "${work_dir}/shadow-authority.log"
held_before="$(psql_command "${container_name}" "SET ROLE go_api_runtime;
  SELECT concat_ws(',',effective,held_reason_code,remaining_requests)
  FROM agent_product_canary_status('${user_one}');" | tail -n 1)"
[[ "${held_before}" == "f,ISOLATION_UNAVAILABLE,0" ]]
opt_out="$(psql_command "${container_name}" "SET ROLE go_api_runtime;
  SELECT concat_ws(',',effective,held_reason_code)
  FROM agent_product_canary_status('${user_two}');" | tail -n 1)"
[[ "${opt_out}" == "f,USER_OPT_OUT" ]]

log "proving immutable exact activation, nonzero fingerprints and replay fences"
psql_command "${container_name}" "
SELECT activation_id FROM agent_product_canary_provision_activation(
  '${activation}','${release_commit}',1,'${admission}','${fp_package}','${fp_runtime}','${fp_plan}',
  '${fp_control}','${fp_root}','${fp_broker}','${fp_project}','${fp_child}','${fp_cron}','${fp_learning}',
  1,clock_timestamp()-interval '1 minute',clock_timestamp()+interval '30 minutes',
  'product-canary-operator','PRODUCT_CANARY_ACTIVATE');" >"${work_dir}/activation.log"
grep -Fxq "${activation}" "${work_dir}/activation.log"
psql_command "${container_name}" "
SELECT (agent_product_canary_provision_activation(
  activation_id,release_commit,policy_revision,admission_id,package_fingerprint,
  runtime_bundle_fingerprint,plan_fingerprint,control_activation_fingerprint,
  root_activation_fingerprint,broker_activation_fingerprint,project_activation_fingerprint,
  child_activation_fingerprint,cron_activation_fingerprint,learning_activation_fingerprint,
  max_requests,valid_from,valid_until,actor_id,reason_code)).activation_id
FROM agent_product_canary_activations WHERE activation_id='${activation}';" \
  >"${work_dir}/activation-replay.log"
grep -Fxq "${activation}" "${work_dir}/activation-replay.log"
assert_rejected postgres activation_replay_drift REPLAY_DETECTED "
  SELECT * FROM agent_product_canary_provision_activation(
  '${activation}','${release_commit}',1,'${admission}','${fp_package}','${fp_runtime}','${fp_plan}',
  '${fp_control}','${fp_root}','${fp_broker}','${fp_project}','${fp_child}','${fp_cron}','${fp_learning}',
  2,(SELECT valid_from FROM agent_product_canary_activations WHERE activation_id='${activation}'),
  (SELECT valid_until FROM agent_product_canary_activations WHERE activation_id='${activation}'),
  'product-canary-operator','PRODUCT_CANARY_ACTIVATE');"
assert_rejected postgres duplicate_activation_fingerprint AGENT_PRODUCT_CANARY_ACTIVATION_INVALID "
  SELECT * FROM agent_product_canary_provision_activation(
  'activation_duplicate123456','${release_commit}',1,'${admission}','${fp_package}','${fp_runtime}','${fp_plan}',
  '${fp_control}','${fp_control}','${fp_broker}','${fp_project}','${fp_child}','${fp_cron}','${fp_learning}',
  1,clock_timestamp(),clock_timestamp()+interval '10 minutes','operator','PRODUCT_CANARY_ACTIVATE');"

ready="$(psql_command "${container_name}" "SET ROLE go_api_runtime;
  SELECT concat_ws(',',activation_id,effective,held_reason_code,remaining_requests)
  FROM agent_product_canary_status('${user_one}');" | tail -n 1)"
[[ "${ready}" == "${activation},t,PRODUCT_CANARY_READY,1" ]]
[[ "$(psql_command "${container_name}" "SET ROLE ${runtime_user};
  SELECT concat_ws(',',stale_claims,pending_terminalizations)
  FROM agent_product_canary_worker_health('${activation}',clock_timestamp());" | tail -n 1)" == "0,0" ]]

log "proving canonical request binding, concurrent API replay, budget and Kill-Switch denial"
psql_command "${container_name}" "SELECT * FROM agent_orchestrator_append_kill_switch(
  'switch_productcanary1234','global','*','deny_new',true,0,'operator','product-canary-drill','PRODUCT_HOLD');" >/dev/null
assert_rejected go_api_runtime kill_switch KILL_SWITCH_ACTIVE "
  SELECT * FROM agent_product_canary_enqueue('product_request_killswitch123456','${user_one}',1,1,'${fp_request}');"
psql_command "${container_name}" "SELECT * FROM agent_orchestrator_append_kill_switch(
  'switch_productcanary1234','global','*','deny_new',false,1,'operator','product-canary-drill','PRODUCT_RESUME');" >/dev/null
assert_rejected go_api_runtime forged_request_fingerprint AGENT_PRODUCT_CANARY_REQUEST_INVALID "
  SELECT * FROM agent_product_canary_enqueue('product_request_forged123456789','${user_one}',1,1,'${fp_snapshot}');"
set +e
psql_command "${container_name}" "SET ROLE go_api_runtime;
  SELECT request_id FROM agent_product_canary_enqueue(
    '${request_candidate_one}','${user_one}',1,1,'${fp_request}');" \
  >"${work_dir}/enqueue-one.log" 2>&1 &
enqueue_one_pid=$!
psql_command "${container_name}" "SET ROLE go_api_runtime;
  SELECT request_id FROM agent_product_canary_enqueue(
    '${request_candidate_two}','${user_one}',1,1,'${fp_request}');" \
  >"${work_dir}/enqueue-two.log" 2>&1 &
enqueue_two_pid=$!
wait "${enqueue_one_pid}"
enqueue_one_status=$?
wait "${enqueue_two_pid}"
enqueue_two_status=$?
set -e
if [[ "${enqueue_one_status}" -ne 0 || "${enqueue_two_status}" -ne 0 ]]; then
  cat "${work_dir}/enqueue-one.log" "${work_dir}/enqueue-two.log" >&2
  exit 1
fi
enqueue_one="$(tail -n 1 "${work_dir}/enqueue-one.log")"
enqueue_two="$(tail -n 1 "${work_dir}/enqueue-two.log")"
[[ "${enqueue_one}" == "${enqueue_two}" ]]
[[ "${enqueue_one}" == "${request_candidate_one}" || "${enqueue_one}" == "${request_candidate_two}" ]]
request_one="${enqueue_one}"
[[ "$(psql_command "${container_name}" "SET ROLE go_api_runtime;
  SELECT request_id FROM agent_product_canary_enqueue(
    'product_request_sequentialreplay1234','${user_one}',1,1,'${fp_request}');" | tail -n 1)" == "${request_one}" ]]
psql_command "${container_name}" "SET ROLE go_api_runtime;
  SELECT generation FROM agent_product_set_shadow_opt_in('${user_two}',0,1,true,'USER_OPT_IN');" >/dev/null
assert_rejected go_api_runtime max_request_budget BUDGET_EXCEEDED "
  SELECT * FROM agent_product_canary_enqueue('product_request_budget1234567890','${user_two}',1,1,'${fp_request_two}');"

log "proving worker lease expiry, read-only health, generation fencing and bounded reconcile"
claim_one="$(psql_command "${container_name}" "SET ROLE ${runtime_user};
  SELECT concat_ws(',',request_id,claim_generation,state) FROM agent_product_canary_worker_claim_requests(
    '${activation}','product-canary-worker',clock_timestamp()-interval '20 seconds',10,1);" | tail -n 1)"
[[ "${claim_one}" == "${request_one},1,claimed" ]]
[[ "$(psql_command "${container_name}" "SET ROLE ${runtime_user};
  SELECT concat_ws(',',stale_claims,pending_terminalizations)
  FROM agent_product_canary_worker_health('${activation}',clock_timestamp());" | tail -n 1)" == "1,0" ]]
[[ "$(psql_command "${container_name}" "SET ROLE ${runtime_user};
  SELECT count(*) FROM agent_product_canary_worker_claim_requests(
    '${activation}','product-canary-worker',clock_timestamp(),300,1);" | tail -n 1)" == "0" ]]
assert_rejected "${runtime_user}" expired_claim_release GENERATION_STALE "
  SELECT agent_product_canary_worker_release_request('${activation}','${request_one}',
    'product-canary-worker',1,'EXPIRED_WORKER',true);"
[[ "$(psql_command "${container_name}" "SET ROLE ${runtime_user};
  SELECT agent_product_canary_worker_reconcile('${activation}',clock_timestamp(),10);" | tail -n 1)" == "1" ]]
[[ "$(psql_command "${container_name}" "SET ROLE ${runtime_user};
  SELECT concat_ws(',',stale_claims,pending_terminalizations)
  FROM agent_product_canary_worker_health('${activation}',clock_timestamp());" | tail -n 1)" == "0,0" ]]
claim_two="$(psql_command "${container_name}" "SET ROLE ${runtime_user};
  SELECT concat_ws(',',request_id,claim_generation,failure_count,state)
  FROM agent_product_canary_worker_claim_requests(
    '${activation}','product-canary-worker',clock_timestamp(),300,1);" | tail -n 1)"
[[ "${claim_two}" == "${request_one},2,1,claimed" ]]
assert_rejected "${runtime_user}" stale_claim GENERATION_STALE "
  SELECT agent_product_canary_worker_release_request('${activation}','${request_one}',
    'product-canary-worker',1,'STALE_WORKER',true);"

log "proving exact product idempotency, Step, receipt and promotion bindings"
wrong_run_id="run_productwrong1234567"
wrong_step_id="step_productwrong123456"
wrong_attempt_id="attempt_productwrong1234"
wrong_snapshot_id="snapshot_productwrong1234"
wrong_receipt="$(canonical_sha256 'neo.agent-product-canary-receipt/v1' "${activation}" \
  "${request_one}" "${wrong_run_id}" "${wrong_attempt_id}" "${fp_snapshot}" "${fp_plan}")"
psql_command "${container_name}" "
INSERT INTO agent_run_snapshots(id,user_id,fingerprint,canonical_snapshot) VALUES
('${wrong_snapshot_id}','${user_one}','${fp_snapshot}','{}');
INSERT INTO agent_runs(id,user_id,snapshot_id,snapshot_fingerprint,idempotency_key,
  request_fingerprint,state,scope_keys,created_at,updated_at,terminal_at)
VALUES('${wrong_run_id}','${user_one}','${wrong_snapshot_id}','${fp_snapshot}',
  'g21.6-product-canary-wrong-request-binding','${fp_request}','canceled',
  ARRAY['global:*','scheduler:*','user:${user_one}','run:${wrong_run_id}'],
  clock_timestamp()-interval '1 second',clock_timestamp(),clock_timestamp());
INSERT INTO agent_steps(id,run_id,user_id,ordinal,kind,state,current_generation,created_at,updated_at,terminal_at)
VALUES('${wrong_step_id}','${wrong_run_id}','${user_one}',0,'product_canary','canceled',1,
  clock_timestamp()-interval '1 second',clock_timestamp(),clock_timestamp());
INSERT INTO agent_attempts(id,run_id,user_id,step_id,generation,state,lease_owner,lease_token_hash,
  lease_expires_at,created_at,updated_at,terminal_at)
VALUES('${wrong_attempt_id}','${wrong_run_id}','${user_one}','${wrong_step_id}',1,
  'canceled','neo-runner-primary','$(printf '1%.0s' {1..64})',clock_timestamp()+interval '1 minute',
  clock_timestamp()-interval '1 second',clock_timestamp(),clock_timestamp());" >/dev/null
assert_rejected "${runtime_user}" wrong_product_idempotency AGENT_PRODUCT_CANARY_RUN_NOT_TERMINAL "
  SELECT * FROM agent_product_canary_worker_complete_request(
  '${activation}','${request_one}','product-canary-worker',2,
  '${wrong_run_id}','${wrong_attempt_id}','${fp_snapshot}','${fp_plan}',
  '${wrong_receipt}','bounded_canary_passed');"
psql_command "${container_name}" "
UPDATE agent_runs SET idempotency_key='g21.6-product-canary-${request_one#product_request_}'
  WHERE id='${wrong_run_id}';
UPDATE agent_steps SET kind='wrong_product_step' WHERE id='${wrong_step_id}';" >/dev/null
assert_rejected "${runtime_user}" wrong_product_step AGENT_PRODUCT_CANARY_RUN_NOT_TERMINAL "
  SELECT * FROM agent_product_canary_worker_complete_request(
  '${activation}','${request_one}','product-canary-worker',2,
  '${wrong_run_id}','${wrong_attempt_id}','${fp_snapshot}','${fp_plan}',
  '${wrong_receipt}','bounded_canary_passed');"
psql_command "${container_name}" "
DELETE FROM agent_runs WHERE id='${wrong_run_id}';
DELETE FROM agent_run_snapshots WHERE id='${wrong_snapshot_id}';
INSERT INTO agent_run_snapshots(id,user_id,fingerprint,canonical_snapshot)
VALUES('snapshot_product123456789','${user_one}','${fp_snapshot}','{}');
INSERT INTO agent_runs(id,user_id,snapshot_id,snapshot_fingerprint,idempotency_key,
  request_fingerprint,state,scope_keys,created_at,updated_at,terminal_at)
VALUES('${run_id}','${user_one}','snapshot_product123456789','${fp_snapshot}',
  'g21.6-product-canary-${request_one#product_request_}','${fp_request}','canceled',
  ARRAY['global:*','scheduler:*','user:${user_one}','run:${run_id}'],
  clock_timestamp()-interval '1 second',clock_timestamp(),clock_timestamp());
INSERT INTO agent_steps(id,run_id,user_id,ordinal,kind,state,current_generation,created_at,updated_at,terminal_at)
VALUES('${step_id}','${run_id}','${user_one}',0,'product_canary','canceled',1,
  clock_timestamp()-interval '1 second',clock_timestamp(),clock_timestamp());
INSERT INTO agent_attempts(id,run_id,user_id,step_id,generation,state,lease_owner,lease_token_hash,
  lease_expires_at,created_at,updated_at,terminal_at)
VALUES('${attempt_id}','${run_id}','${user_one}','${step_id}',1,
  'canceled','neo-runner-primary','$(printf '1%.0s' {1..64})',clock_timestamp()+interval '1 minute',
  clock_timestamp()-interval '1 second',clock_timestamp(),clock_timestamp());" >/dev/null
[[ "$(psql_command "${container_name}" "SET ROLE ${runtime_user};
  SELECT concat_ws(',',stale_claims,pending_terminalizations)
  FROM agent_product_canary_worker_health('${activation}',clock_timestamp());" | tail -n 1)" == "0,1" ]]
fp_receipt="$(canonical_sha256 'neo.agent-product-canary-receipt/v1' "${activation}" \
  "${request_one}" "${run_id}" "${attempt_id}" "${fp_snapshot}" "${fp_plan}")"
assert_rejected "${runtime_user}" forged_receipt_binding AGENT_PRODUCT_CANARY_RECEIPT_BINDING_INVALID "
  SELECT * FROM agent_product_canary_worker_complete_request(
  '${activation}','${request_one}','product-canary-worker',2,
  '${run_id}','${attempt_id}','${fp_snapshot}','${fp_plan}',
  '${fp_request}','bounded_canary_passed');"
psql_command "${container_name}" "SET ROLE ${runtime_user};
SELECT request_id FROM agent_product_canary_worker_complete_request(
  '${activation}','${request_one}','product-canary-worker',2,
  '${run_id}','${attempt_id}','${fp_snapshot}','${fp_plan}',
  '${fp_receipt}','bounded_canary_passed');
SELECT request_id FROM agent_product_canary_worker_complete_request(
  '${activation}','${request_one}','product-canary-worker',2,
  '${run_id}','${attempt_id}','${fp_snapshot}','${fp_plan}',
  '${fp_receipt}','bounded_canary_passed');" >"${work_dir}/complete.log"
[[ "$(grep -Fxc "${request_one}" "${work_dir}/complete.log")" == "2" ]]
[[ "$(psql_command "${container_name}" "SET ROLE ${runtime_user};
  SELECT concat_ws(',',stale_claims,pending_terminalizations)
  FROM agent_product_canary_worker_health('${activation}',clock_timestamp());" | tail -n 1)" == "0,0" ]]
assert_rejected "${runtime_user}" receipt_drift REPLAY_DETECTED "
  SELECT * FROM agent_product_canary_worker_complete_request(
  '${activation}','${request_one}','product-canary-worker',2,
  '${run_id}','${attempt_id}','${fp_snapshot}','${fp_plan}',
  '${fp_request}','bounded_canary_passed');"
psql_command "${container_name}" "
SELECT promotion_id FROM agent_product_canary_record_promotion(
  'promotion_1234567890abcdef','${activation}','${request_one}','${release_commit}',
  '${fp_closure}','${fp_receipt}','PROMOTION_READY','release-operator','FINAL_CLOSURE_READY');
SELECT promotion_id FROM agent_product_canary_record_promotion(
  'promotion_1234567890abcdef','${activation}','${request_one}','${release_commit}',
  '${fp_closure}','${fp_receipt}','PROMOTION_READY','release-operator','FINAL_CLOSURE_READY');" \
  >"${work_dir}/promotion.log"
[[ "$(grep -Fxc 'promotion_1234567890abcdef' "${work_dir}/promotion.log")" == "2" ]]
assert_rejected go_api_runtime api_promotion_denied "permission denied" "
  SELECT * FROM agent_product_canary_record_promotion(
  'promotion_api1234567890','${activation}','${request_one}','${release_commit}',
  '${fp_snapshot}','${fp_receipt}','PROMOTION_READY','api','FINAL_CLOSURE_READY');"
assert_rejected "${runtime_user}" worker_promotion_denied "permission denied" "
  SELECT * FROM agent_product_canary_record_promotion(
  'promotion_worker1234567','${activation}','${request_one}','${release_commit}',
  '${fp_snapshot}','${fp_receipt}','PROMOTION_READY','worker','FINAL_CLOSURE_READY');"
assert_rejected postgres promotion_drift REPLAY_DETECTED "
  SELECT * FROM agent_product_canary_record_promotion(
  'promotion_1234567890abcdef','${activation}','${request_one}','${release_commit}',
  '${fp_snapshot}','${fp_receipt}','PROMOTION_READY','release-operator','FINAL_CLOSURE_READY');"
assert_rejected postgres immutable_receipt AGENT_PRODUCT_FACT_IMMUTABLE "
  UPDATE agent_product_canary_receipts SET outcome='bounded_canary_passed' WHERE request_id='${request_one}';"

log "proving content-free schema and dump/restore authority"
psql_command "${container_name}" "
DO \$\$
BEGIN
  IF EXISTS(SELECT 1 FROM information_schema.columns
    WHERE table_schema=current_schema()
      AND table_name IN ('agent_product_canary_activations','agent_product_canary_requests',
        'agent_product_canary_receipts','agent_product_canary_promotions')
      AND lower(column_name) ~ '(prompt|body|argument|secret|output|workspace|object_key)') THEN
    RAISE EXCEPTION 'Product canary content-bearing column found';
  END IF;
END
\$\$;" >/dev/null
docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
  pg_dump --no-owner --no-privileges -U "${database_user}" -d "${database_name}" \
  >"${work_dir}/authority.sql"
source_counts="$(psql_command "${container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_product_canary_activations),
  (SELECT count(*) FROM agent_product_canary_requests),
  (SELECT count(*) FROM agent_product_canary_receipts),
  (SELECT count(*) FROM agent_product_canary_promotions));")"
start_database "${restore_container_name}"
docker exec -i -e "PGPASSWORD=${database_password}" "${restore_container_name}" \
  psql --set=ON_ERROR_STOP=1 --no-psqlrc -U "${database_user}" -d "${database_name}" \
  <"${work_dir}/authority.sql" >/dev/null
restore_counts="$(psql_command "${restore_container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_product_canary_activations),
  (SELECT count(*) FROM agent_product_canary_requests),
  (SELECT count(*) FROM agent_product_canary_receipts),
  (SELECT count(*) FROM agent_product_canary_promotions));")"
[[ "${source_counts}" == "${restore_counts}" ]]

log "proving dirty 095 down refusal, then clean 095/096 replay"
run_migrate down >"${work_dir}/peel-096-chat-agent-event-tail.log" 2>&1
grep -Fq "down 096_chat_agent_event_log" "${work_dir}/peel-096-chat-agent-event-tail.log"
set +e
run_migrate down >"${work_dir}/dirty-down.log" 2>&1
dirty_status=$?
set -e
[[ "${dirty_status}" -ne 0 ]]
grep -Fq "AGENT_PRODUCT_CANARY_DOWN_REQUIRES_EMPTY" "${work_dir}/dirty-down.log"
psql_command "${container_name}" "
TRUNCATE agent_product_canary_promotions,agent_product_canary_receipts,
  agent_product_canary_requests,agent_product_canary_activations;
REVOKE agent_product_canary_worker,agent_orchestrator_runtime,agent_runner_control FROM ${runtime_user};
DROP ROLE ${runtime_user};" >/dev/null
run_migrate down >"${work_dir}/clean-down.log" 2>&1
grep -Fq "down 095_agent_product_canary_activation" "${work_dir}/clean-down.log"
run_migrate up >"${work_dir}/clean-reup.log" 2>&1
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/clean-reup.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/clean-reup.log"
[[ "$(psql_command "${container_name}" 'SELECT max(version) FROM schema_migrations')" == "96" ]]

log "passed (fresh/replay, canonical/concurrent enqueue, exact LOGIN ACL, read-only health, lease/reconcile, terminal receipt, promotion, dump/restore, guarded down/up)"
