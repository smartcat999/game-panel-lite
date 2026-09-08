CREATE FUNCTION protect_prepaid_order_terms() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'DELETE' THEN RAISE EXCEPTION 'order history cannot be deleted'; END IF;
 IF ROW(NEW.id,NEW.organization_id,NEW.server_id,NEW.revision_id,NEW.placement_epoch,NEW.quote,NEW.created_at_ms,NEW.expires_at_ms,NEW.actor_id,NEW.idempotency_key,NEW.request_hash)
 IS DISTINCT FROM ROW(OLD.id,OLD.organization_id,OLD.server_id,OLD.revision_id,OLD.placement_epoch,OLD.quote,OLD.created_at_ms,OLD.expires_at_ms,OLD.actor_id,OLD.idempotency_key,OLD.request_hash)
 THEN RAISE EXCEPTION 'order terms are immutable'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER prepaid_order_terms_immutable BEFORE UPDATE OR DELETE ON prepaid_orders
 FOR EACH ROW EXECUTE FUNCTION protect_prepaid_order_terms();
