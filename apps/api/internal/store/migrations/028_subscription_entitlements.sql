ALTER TABLE global_server_entitlements DROP CONSTRAINT IF EXISTS global_server_entitlements_source_kind_check;
ALTER TABLE global_server_entitlements ADD CONSTRAINT global_server_entitlements_source_kind_check CHECK(source_kind IN ('operator','subscription','prepaid_subscription'));
ALTER TABLE global_entitlement_changes DROP CONSTRAINT IF EXISTS global_entitlement_changes_source_kind_check;
ALTER TABLE global_entitlement_changes ADD CONSTRAINT global_entitlement_changes_source_kind_check CHECK(source_kind IN ('operator','subscription','prepaid_subscription'));
