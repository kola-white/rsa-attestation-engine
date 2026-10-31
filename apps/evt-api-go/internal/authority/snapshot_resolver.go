package authority

import (
	"context"
	"errors"
	"time"
)

var errInvalidAuthoritySnapshot = errors.New("invalid authority snapshot")

// AuthoritySnapshotEntry associates an authorization record with its
// complete lifecycle journal.
type AuthoritySnapshotEntry struct {
	Record  AuthorizationRecord
	Journal AuthorizationJournal
}

// AuthoritySnapshot represents a coherent authoritative registry view.
// Complete is an assurance supplied by the trusted snapshot producer.
type AuthoritySnapshot struct {
	Complete          bool
	RecordingPosition int64
	RecordedThrough   time.Time
	Entries           []AuthoritySnapshotEntry
}

// SnapshotAuthorityResolver resolves grants from an immutable snapshot.
type SnapshotAuthorityResolver struct {
	grants []AuthorityGrant
}

// NewSnapshotAuthorityResolver validates and projects the snapshot once.
// The resolver retains derived grants, not mutable record or journal pointers.
func NewSnapshotAuthorityResolver(
	snapshot AuthoritySnapshot,
) (*SnapshotAuthorityResolver, error) {
	if !snapshot.Complete ||
		!canonicalAuthorityTimestamp(snapshot.RecordedThrough) {
		return nil, errInvalidAuthoritySnapshot
	}

	if len(snapshot.Entries) == 0 {
		return &SnapshotAuthorityResolver{}, nil
	}

	seen := make(map[string]bool)
	var grants []AuthorityGrant

	for _, entry := range snapshot.Entries {
		id := entry.Record.AuthorizationID

		if id == "" || seen[id] {
			return nil, errInvalidAuthoritySnapshot
		}
		seen[id] = true

		for _, event := range entry.Journal.Events() {
			if event.AuthorizationID != id ||
				!canonicalAuthorityTimestamp(event.RecordedAt) ||
				event.RecordedAt.After(snapshot.RecordedThrough) {
				return nil, errInvalidAuthoritySnapshot
			}
		}

		projected, err := ProjectAuthorityGrants(
			entry.Record,
			&entry.Journal,
			snapshot.RecordedThrough,
		)
		if err != nil {
			return nil, errors.Join(errInvalidAuthoritySnapshot, err)
		}

		grants = append(grants, projected...)
	}

	return &SnapshotAuthorityResolver{
		grants: grants,
	}, nil
}

// ResolveAuthorityGrants retrieves all historical grants matching issuer
// and claim type. Authority-subject matching belongs to the evaluator.
func (r *SnapshotAuthorityResolver) ResolveAuthorityGrants(
	ctx context.Context,
	issuerID string,
	claimType string,
) ([]AuthorityGrant, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if r == nil {
		return nil, errInvalidAuthoritySnapshot
	}

	var matching []AuthorityGrant

	for _, grant := range r.grants {
		if grant.IssuerID != issuerID || grant.ClaimType != claimType {
			continue
		}

		copyGrant := grant
		if grant.ValidUntil != nil {
			end := *grant.ValidUntil
			copyGrant.ValidUntil = &end
		}

		matching = append(matching, copyGrant)
	}

	return matching, nil
}
