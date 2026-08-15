DO $guard$
BEGIN
  IF EXISTS(SELECT 1 FROM agent_product_canary_activations)
     OR EXISTS(SELECT 1 FROM agent_product_canary_requests)
     OR EXISTS(SELECT 1 FROM agent_product_canary_receipts)
     OR EXISTS(SELECT 1 FROM agent_product_canary_promotions) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_PRODUCT_CANARY_DOWN_REQUIRES_EMPTY';
  END IF;
END
$guard$;

REVOKE EXECUTE ON FUNCTION agent_product_canary_status(UUID),
  agent_product_canary_enqueue(TEXT,UUID,BIGINT,BIGINT,TEXT) FROM go_api_runtime;
REVOKE EXECUTE ON FUNCTION agent_product_canary_worker_get_activation(TEXT),
  agent_product_canary_worker_health(TEXT,TIMESTAMPTZ),
  agent_product_canary_worker_claim_requests(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_product_canary_worker_complete_request(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_product_canary_worker_release_request(TEXT,TEXT,TEXT,BIGINT,TEXT,BOOLEAN),
  agent_product_canary_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER)
  FROM agent_product_canary_worker;

DROP FUNCTION agent_product_canary_record_promotion(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_product_canary_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_product_canary_worker_release_request(TEXT,TEXT,TEXT,BIGINT,TEXT,BOOLEAN);
DROP FUNCTION agent_product_canary_worker_complete_request(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_product_canary_worker_claim_requests(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER);
DROP FUNCTION agent_product_canary_worker_health(TEXT,TIMESTAMPTZ);
DROP FUNCTION agent_product_canary_worker_get_activation(TEXT);
DROP FUNCTION agent_product_canary_enqueue(TEXT,UUID,BIGINT,BIGINT,TEXT);
DROP FUNCTION agent_product_canary_status(UUID);
DROP FUNCTION agent_product_canary_disable_activation(TEXT,TEXT,TEXT);
DROP FUNCTION agent_product_canary_provision_activation(TEXT,TEXT,BIGINT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER,TIMESTAMPTZ,TIMESTAMPTZ,TEXT,TEXT);

DROP TRIGGER trg_agent_product_canary_promotion_immutable ON agent_product_canary_promotions;
DROP TRIGGER trg_agent_product_canary_receipt_immutable ON agent_product_canary_receipts;
DROP TRIGGER trg_agent_product_canary_request_guard ON agent_product_canary_requests;
DROP TRIGGER trg_agent_product_canary_activation_guard ON agent_product_canary_activations;
DROP TABLE agent_product_canary_promotions;
DROP TABLE agent_product_canary_receipts;
DROP TABLE agent_product_canary_requests;
DROP TABLE agent_product_canary_activations;
DROP FUNCTION agent_product_canary_request_guard();
DROP FUNCTION agent_product_canary_activation_guard();

DROP ROLE agent_product_canary_worker;
