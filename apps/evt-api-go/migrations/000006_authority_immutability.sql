CREATE FUNCTION reject_authority_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'Authority records are append-only: % on % is prohibited',
        TG_OP, TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER authority_authorizations_immutable
BEFORE UPDATE OR DELETE ON authority_authorizations
FOR EACH ROW EXECUTE FUNCTION reject_authority_mutation();

CREATE TRIGGER authority_adjudications_immutable
BEFORE UPDATE OR DELETE ON authority_adjudications
FOR EACH ROW EXECUTE FUNCTION reject_authority_mutation();

CREATE TRIGGER authority_journal_events_immutable
BEFORE UPDATE OR DELETE ON authority_journal_events
FOR EACH ROW EXECUTE FUNCTION reject_authority_mutation();
