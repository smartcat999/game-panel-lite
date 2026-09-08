CREATE FUNCTION reject_prepaid_payment_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'payment history is immutable';
END;
$$;
CREATE TRIGGER prepaid_captures_immutable BEFORE UPDATE OR DELETE ON prepaid_payment_captures
 FOR EACH ROW EXECUTE FUNCTION reject_prepaid_payment_mutation();
CREATE TRIGGER prepaid_events_immutable BEFORE UPDATE OR DELETE ON prepaid_payment_events
 FOR EACH ROW EXECUTE FUNCTION reject_prepaid_payment_mutation();
