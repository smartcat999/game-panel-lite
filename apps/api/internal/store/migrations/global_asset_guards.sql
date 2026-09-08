CREATE FUNCTION reject_global_asset_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'published asset identity and versions are immutable';
END;
$$;
CREATE TRIGGER global_assets_immutable BEFORE UPDATE OR DELETE ON global_assets
FOR EACH ROW EXECUTE FUNCTION reject_global_asset_mutation();
CREATE TRIGGER global_asset_versions_immutable BEFORE UPDATE OR DELETE ON global_asset_versions
FOR EACH ROW EXECUTE FUNCTION reject_global_asset_mutation();
