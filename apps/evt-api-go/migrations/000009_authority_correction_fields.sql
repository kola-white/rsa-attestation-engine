ALTER TABLE authority_journal_events
ADD CONSTRAINT authority_journal_supported_correction_field
CHECK (
    corrects_event_id IS NULL
    OR corrected_field = 'EffectiveAt'
);
