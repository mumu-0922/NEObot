UPDATE mcp_servers AS server
SET auth_config = jsonb_set(
      jsonb_set(
        server.auth_config,
        '{metadata,marketplace,deploymentHash}',
        to_jsonb('22b235834a14b617480cc92dd0f6f6c7587cb399880c666773135971767bc6e2'::text),
        true
      ),
      '{metadata,legacyArtifactRepair}',
      to_jsonb('081'::text),
      true
    ),
    updated_at = now()
WHERE server.deleted_at IS NULL
  AND server.transport = 'stdio'
  AND server.endpoint_url = 'runner://marketplace-upstash-context7-2.2.0'
  AND server.auth_type = 'none'
  AND server.auth_config #>> '{metadata,runnerArtifactId}' =
    'marketplace-upstash-context7-2.2.0'
  AND server.auth_config #>> '{metadata,marketplace,provider}' = 'lobehub'
  AND server.auth_config #>> '{metadata,marketplace,identifier}' =
    'upstash-context7'
  AND server.auth_config #>> '{metadata,marketplace,version}' = '2.2.0'
  AND server.auth_config #>> '{metadata,marketplace,deploymentHash}' =
    '20a578fff586f03151f2f2f6aa97ea331d3985f8e93ce068f0f59e984cc4964d';
