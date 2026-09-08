CREATE FUNCTION reject_prepaid_plan_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'published plan version is immutable';
END;
$$;
CREATE TRIGGER prepaid_plan_versions_immutable BEFORE UPDATE OR DELETE ON prepaid_plan_versions
 FOR EACH ROW EXECUTE FUNCTION reject_prepaid_plan_mutation();
