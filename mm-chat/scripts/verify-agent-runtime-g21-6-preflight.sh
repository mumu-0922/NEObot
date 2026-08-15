#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
preflight="${script_dir}/preflight-single-server.sh"
policy="${project_dir}/config/agent-runner/production-policy.json"
fixture_dir="${project_dir}/docs/contracts/fixtures/agent-runtime"
export_dir="${G21_PREFLIGHT_EXPORT_DIR:-}"
if [[ -n "${export_dir}" ]]; then
  [[ "${export_dir}" == /* ]] || { echo 'G21.6 preflight: export directory must be absolute' >&2; exit 1; }
  mkdir -p "${export_dir}"
  work_dir="$(cd -- "${export_dir}" && pwd -P)"
  cleanup(){ :; }
else
  work_dir="$(mktemp -d)"
  cleanup(){ find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true; rmdir "${work_dir}" 2>/dev/null || true; }
fi
trap cleanup EXIT INT TERM

for command in bash date docker jq openssl python3 sed sha256sum; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "G21.6 preflight: ${command} is required" >&2
    exit 1
  }
done

# Reuse the executable preflight fixture builders rather than maintaining a
# second, weaker copy of the G21.0-G21.5 exact-host configuration contract.
prior_dir="${work_dir}/g21-0-4"
workers_dir="${work_dir}/g21-5"
G21_PREFLIGHT_EXPORT_DIR="${prior_dir}" \
  bash "${script_dir}/verify-agent-child-canary-preflight.sh" >/dev/null
G21_PREFLIGHT_BASE_ENV="${prior_dir}/enabled.env" \
G21_PREFLIGHT_EXPORT_DIR="${workers_dir}" \
  bash "${script_dir}/verify-agent-runtime-g21-5-preflight.sh" >/dev/null
base_env="${workers_dir}/enabled-true-true.env"
[[ -s "${base_env}" ]] || { echo 'G21.6 preflight: prior-stage fixture export failed' >&2; exit 1; }

product_dir="${work_dir}/product"
mkdir "${product_dir}"
cat >"${product_dir}/client.cnf" <<'EOF_CERT'
[req]
distinguished_name=dn
prompt=no
[dn]
CN=spiffe://neo-chat/agent-runtime-product-canary
EOF_CERT
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -sha256 \
  -config "${product_dir}/client.cnf" -keyout "${product_dir}/client.key" \
  -out "${product_dir}/client.crt" >/dev/null 2>&1
cat >"${product_dir}/server-ca.cnf" <<'EOF_CERT'
[req]
distinguished_name=dn
prompt=no
[dn]
CN=neo-runner-product-canary-test-ca
EOF_CERT
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -sha256 \
  -config "${product_dir}/server-ca.cnf" -keyout "${product_dir}/server-ca.key" \
  -out "${product_dir}/server-ca.crt" >/dev/null 2>&1
openssl genpkey -algorithm Ed25519 -out "${product_dir}/authority.pem" >/dev/null 2>&1
openssl pkey -in "${product_dir}/authority.pem" -outform DER -out "${product_dir}/authority.der"
openssl pkey -in "${product_dir}/authority.pem" -pubout -outform DER \
  -out "${product_dir}/authority-public.der"
python3 - "${product_dir}/authority.der" "${product_dir}/authority-public.der" \
  "${product_dir}/authority-private-key" "${product_dir}/authority-public-key" <<'PY'
import base64,sys
private=open(sys.argv[1],'rb').read()[-32:]
public=open(sys.argv[2],'rb').read()[-32:]
open(sys.argv[3],'w').write(base64.urlsafe_b64encode(private+public).decode().rstrip('=')+'\n')
open(sys.argv[4],'w').write(base64.urlsafe_b64encode(public).decode().rstrip('=')+'\n')
PY

activation_id='activation_0123456789abcdef'
endpoint='https://10.0.0.8:9443/internal/neo-runner/v1/rpc'
commit='1111111111111111111111111111111111111111'
python3 - "${project_dir}" "${base_env}" "${product_dir}" \
  "${activation_id}" "${endpoint}" "${commit}" <<'PY'
import hashlib,json,os,sys
from datetime import datetime,timedelta,timezone
from pathlib import Path

project,base_env,work=map(Path,sys.argv[1:4])
activation_id,endpoint,commit=sys.argv[4:]
values={}
for raw in base_env.read_text().splitlines():
    line=raw.strip()
    if not line or line.startswith('#') or '=' not in line:
        continue
    key,value=line.split('=',1)
    values[key]=value
fp=lambda path:'sha256:'+hashlib.sha256(Path(path).read_bytes()).hexdigest()
label=lambda value:'sha256:'+hashlib.sha256(value.encode()).hexdigest()
now=datetime.now(timezone.utc).replace(microsecond=0)
fmt=lambda value:value.isoformat().replace('+00:00','Z')

plan=json.loads((project/'docs/contracts/fixtures/agent-runtime/neo-agent-product-canary-plan.valid.json').read_text())
nonzero=label('g21.6-preflight-product-plan')
def replace(value):
    if isinstance(value,dict): return {key:replace(item) for key,item in value.items()}
    if isinstance(value,list): return [replace(item) for item in value]
    if value=='sha256:'+'0'*64: return nonzero
    return value
plan=replace(plan)
plan['activationId']=activation_id
plan['sandbox']['image']='registry.invalid/neo/product-canary@'+nonzero
(work/'plan.json').write_text(json.dumps(plan,separators=(',',':'))+'\n')

manifest=json.loads(Path(values['AGENT_CHILD_CANARY_RELEASE_MANIFEST_SOURCE']).read_text())
manifest['runnerVersion']='g21.6-preflight-test'
(work/'release-manifest.json').write_text(json.dumps(manifest,separators=(',',':'))+'\n')

record=json.loads((project/'docs/contracts/fixtures/agent-runtime/neo-agent-product-canary-activation.valid.json').read_text())
record['evidenceClass']='production'
record['release']={
  'gitCommit':commit,'migrationHead':95,
  'runnerManifestSha256':fp(work/'release-manifest.json'),
  'runnerBinarySha256':label('g21.6-preflight-runner'),
  'operationsPolicySha256':fp(project/'config/agent-runner/production-policy.json'),
}
record['target']={'deploymentFingerprint':label('g21.6-preflight-deployment'),'runnerId':'neo-runner-primary'}
endpoint_fp='sha256:'+hashlib.sha256(b'neo-agent-runner-endpoint-v1\0'+endpoint.encode()).hexdigest()
record['wiring']={
  'activationId':activation_id,'endpointSha256':endpoint_fp,
  'clientCertificateSha256':fp(work/'client.crt'),
  'serverCASha256':fp(work/'server-ca.crt'),'serverName':'neo-runner.internal',
  'callerIdentity':'spiffe://neo-chat/agent-runtime-product-canary',
  'canaryPlanSha256':fp(work/'plan.json'),
  'authorityPublicKeySha256':fp(work/'authority-public-key'),
}
prerequisite_sources={
  'controlPlane':'AGENT_PRODUCTION_ACTIVATION_SOURCE',
  'rootRun':'AGENT_ROOT_CANARY_ACTIVATION_SOURCE',
  'brokerArtifact':'AGENT_BROKER_CANARY_ACTIVATION_SOURCE',
  'projectMutation':'AGENT_PROJECT_CANARY_ACTIVATION_SOURCE',
  'depthOneChild':'AGENT_CHILD_CANARY_ACTIVATION_SOURCE',
  'cronWorker':'AGENT_CRON_WORKER_ACTIVATION_SOURCE',
  'draftLearning':'AGENT_DRAFT_LEARNING_WORKER_ACTIVATION_SOURCE',
}
record['prerequisites']={key:fp(values[source]) for key,source in prerequisite_sources.items()}
record['window']={
  'startedAt':fmt(now-timedelta(minutes=5)),
  'completedAt':fmt(now-timedelta(minutes=2)),
  'expiresAt':fmt(now+timedelta(hours=1)),
}
for index,check in enumerate(record['checks']):
    check.update(result='passed',observedAt=fmt(now-timedelta(minutes=3)),
                 evidenceSha256=label(f'g21.6-preflight-check-{index}'),detailCode='PASS')
record['cleanup']={key:0 for key in record['cleanup']}
record['review']={
  'decision':'approved','reviewedAt':fmt(now-timedelta(minutes=1)),
  'reviewerFingerprint':label('g21.6-preflight-reviewer'),
}
(work/'activation.json').write_text(json.dumps(record,separators=(',',':'))+'\n')
for path in work.iterdir():
    if path.is_file(): os.chmod(path,0o600)
PY

valid_env="${work_dir}/enabled.env"
sed \
  -e 's|change-me-agent-product-canary-postgres|test-agent-product-canary-password|g' \
  -e 's|^AGENT_PRODUCT_CANARY_ENABLED=false$|AGENT_PRODUCT_CANARY_ENABLED=true|' \
  -e "s|^AGENT_PRODUCT_CANARY_ACTIVATION_ID=.*|AGENT_PRODUCT_CANARY_ACTIVATION_ID=${activation_id}|" \
  -e "s|^AGENT_PRODUCT_CANARY_RUNNER_URL=.*|AGENT_PRODUCT_CANARY_RUNNER_URL=${endpoint}|" \
  -e "s|^AGENT_PRODUCT_CANARY_CLIENT_CERT_SOURCE=.*|AGENT_PRODUCT_CANARY_CLIENT_CERT_SOURCE=${product_dir}/client.crt|" \
  -e "s|^AGENT_PRODUCT_CANARY_CLIENT_KEY_SOURCE=.*|AGENT_PRODUCT_CANARY_CLIENT_KEY_SOURCE=${product_dir}/client.key|" \
  -e "s|^AGENT_PRODUCT_CANARY_SERVER_CA_SOURCE=.*|AGENT_PRODUCT_CANARY_SERVER_CA_SOURCE=${product_dir}/server-ca.crt|" \
  -e "s|^AGENT_PRODUCT_CANARY_RELEASE_MANIFEST_SOURCE=.*|AGENT_PRODUCT_CANARY_RELEASE_MANIFEST_SOURCE=${product_dir}/release-manifest.json|" \
  -e "s|^AGENT_PRODUCT_CANARY_PRODUCTION_POLICY_SOURCE=.*|AGENT_PRODUCT_CANARY_PRODUCTION_POLICY_SOURCE=${policy}|" \
  -e "s|^AGENT_PRODUCT_CANARY_ACTIVATION_SOURCE=.*|AGENT_PRODUCT_CANARY_ACTIVATION_SOURCE=${product_dir}/activation.json|" \
  -e "s|^AGENT_PRODUCT_CANARY_PLAN_SOURCE=.*|AGENT_PRODUCT_CANARY_PLAN_SOURCE=${product_dir}/plan.json|" \
  -e "s|^AGENT_PRODUCT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE=.*|AGENT_PRODUCT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE=${product_dir}/authority-private-key|" \
  -e "s|^AGENT_PRODUCT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE=.*|AGENT_PRODUCT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE=${product_dir}/authority-public-key|" \
  -e "s|^AGENT_PRODUCT_CANARY_RELEASE_GIT_COMMIT=.*|AGENT_PRODUCT_CANARY_RELEASE_GIT_COMMIT=${commit}|" \
  "${base_env}" >"${valid_env}"
chmod 600 "${valid_env}"

bash "${preflight}" "${valid_env}" >/dev/null

assert_rejected(){
  local file="$1" expected="$2" output
  if output="$(bash "${preflight}" "${file}" 2>&1)"; then
    echo "G21.6 preflight: accepted negative ${file}" >&2
    exit 1
  fi
  [[ "${output}" == *"${expected}"* ]] || { printf '%s\n' "${output}" >&2; exit 1; }
}

missing_prerequisite="${work_dir}/missing-prerequisite.env"
sed 's|^AGENT_DRAFT_LEARNING_WORKER_ENABLED=true$|AGENT_DRAFT_LEARNING_WORKER_ENABLED=false|' \
  "${valid_env}" >"${missing_prerequisite}"
chmod 600 "${missing_prerequisite}"
assert_rejected "${missing_prerequisite}" 'requires every G21.0-G21.5 stage flag'

reused_key="${work_dir}/reused-key.env"
sed "s|^AGENT_PRODUCT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE=.*|AGENT_PRODUCT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE=${prior_dir}/child-authority-public-key|" \
  "${valid_env}" >"${reused_key}"
chmod 600 "${reused_key}"
assert_rejected "${reused_key}" 'must not reuse prior Runner or canary key material'

held_activation="${work_dir}/held-activation.env"
cp "${fixture_dir}/neo-agent-product-canary-activation.valid.json" "${work_dir}/held-activation.json"
chmod 600 "${work_dir}/held-activation.json"
sed "s|^AGENT_PRODUCT_CANARY_ACTIVATION_SOURCE=.*|AGENT_PRODUCT_CANARY_ACTIVATION_SOURCE=${work_dir}/held-activation.json|" \
  "${valid_env}" >"${held_activation}"
chmod 600 "${held_activation}"
assert_rejected "${held_activation}" 'is not READY for G21.6'

forbidden_credential="${work_dir}/forbidden-credential.env"
cp "${valid_env}" "${forbidden_credential}"
printf '%s\n' 'AGENT_PRODUCT_CANARY_MCP_TOKEN=forbidden-value' >>"${forbidden_credential}"
chmod 600 "${forbidden_credential}"
assert_rejected "${forbidden_credential}" 'must not receive generic execution or effect credentials'

compose_json="${work_dir}/compose.json"
docker compose --env-file "${valid_env}" -f "${project_dir}/compose.yml" \
  --profile agent-runtime-product-canary config --format json >"${compose_json}"
python3 - "${compose_json}" <<'PY'
import json,sys
service=json.load(open(sys.argv[1]))['services']['agent-runtime-product-canary']
assert service['profiles']==['agent-runtime-product-canary']
assert set(service['networks'])=={'private'}
assert not service.get('ports') and not service.get('secrets')
assert len(service.get('volumes',[]))==9
assert all(item.get('read_only') is True for item in service['volumes'])
assert all(item.get('bind',{}).get('create_host_path') is False for item in service['volumes'])
environment=service['environment']
assert environment['AGENT_PRODUCT_CANARY_ENABLED']=='true'
assert environment['AGENT_PRODUCT_CANARY_CLIENT_IDENTITY']=='spiffe://neo-chat/agent-runtime-product-canary'
for key in ('AGENT_RUNNER_CONTROL_ENABLED','AGENT_ROOT_RUN_CANARY_ENABLED',
            'AGENT_BROKER_ARTIFACT_CANARY_ENABLED','AGENT_PROJECT_MUTATION_CANARY_ENABLED',
            'AGENT_CHILD_CANARY_ENABLED','AGENT_CRON_WORKER_ENABLED',
            'AGENT_DRAFT_LEARNING_WORKER_ENABLED'):
    assert environment[key]=='true',key
for marker in ('MCP_','S3_','PROVIDER_','VAULT_','REDIS_','BROKER_','PROJECT_','CHILD_'):
    assert not any(key.startswith('AGENT_PRODUCT_CANARY_'+marker) for key in environment),marker
PY

production_json="${work_dir}/compose-production.json"
docker compose --env-file "${valid_env}" -f "${project_dir}/compose.yml" \
  -f "${project_dir}/compose.production.yml" \
  --profile agent-runtime-product-canary config --format json >"${production_json}"
python3 - "${production_json}" <<'PY'
import json,re,sys
service=json.load(open(sys.argv[1]))['services']['agent-runtime-product-canary']
assert not service.get('build')
assert re.fullmatch(r'[^@\s]+@sha256:[0-9a-f]{64}',service['image'])
PY

printf '%s\n' 'Agent Runtime G21.6 exact enabled preflight verification: passed.'
