CREATE FUNCTION reject_entitlement_change_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'entitlement change history is immutable';
END;
$$;
CREATE TRIGGER global_entitlement_changes_immutable BEFORE UPDATE OR DELETE ON global_entitlement_changes
 FOR EACH ROW EXECUTE FUNCTION reject_entitlement_change_mutation();
