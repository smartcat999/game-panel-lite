CREATE FUNCTION reject_server_revision_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'server revisions are immutable';
END;
$$;
CREATE TRIGGER server_revisions_immutable BEFORE UPDATE OR DELETE ON server_revisions
FOR EACH ROW EXECUTE FUNCTION reject_server_revision_mutation();
