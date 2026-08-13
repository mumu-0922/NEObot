REVOKE ALL ON TABLE assistant_market_admissions, assistant_library_entries
  FROM go_api_runtime;

DROP TABLE IF EXISTS assistant_market_admissions;
DROP TABLE IF EXISTS assistant_library_entries;
