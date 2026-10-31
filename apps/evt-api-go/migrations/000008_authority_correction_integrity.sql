ALTER TABLE authority_journal_events
ADD CONSTRAINT authority_journal_event_scope_unique
UNIQUE (event_id, authorization_id);

ALTER TABLE authority_journal_events
ADD CONSTRAINT authority_journal_correction_reference_fk
FOREIGN KEY (corrects_event_id, authorization_id)
REFERENCES authority_journal_events (
    event_id,
    authorization_id
);
