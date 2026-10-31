ALTER TABLE authority_journal_events
ADD CONSTRAINT authority_journal_valid_event_type
CHECK (
    event_type IN (
        'SUBMIT',
        'APPROVE',
        'REJECT',
        'ACTIVATE',
        'SUSPEND',
        'REINSTATE',
        'EXPIRE',
        'REVOKE'
    )
);
