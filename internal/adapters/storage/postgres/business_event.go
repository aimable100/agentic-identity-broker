package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Append uses the pool even when ctx carries an already committed state-change transaction.
func (a *Adapter) Append(ctx context.Context, event *model.BusinessEvent) (err error) {
	const operation = "Append"
	ctx, finish := startStorageSpan(ctx, "storage.append.business_event", operation)
	defer func() { finish(err) }()
	if a.db == nil {
		return storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	if event == nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "business event cannot be nil")
	}
	if err := event.Validate(); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid business event")
	}
	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()
	if err := execCtx.Err(); err != nil {
		return businessEventStorageError(operation, err, "failed to append business event")
	}
	var attributes any = "{}"
	if event.Attributes != nil {
		attributes, err = json.Marshal(event.Attributes)
		if err != nil {
			return storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid business event attributes")
		}
	}
	var recordedAt time.Time
	err = sqlx.GetContext(execCtx, a.db, &recordedAt, `INSERT INTO business_events
		(id, event, occurred_at, principal, actor, trace_id, attributes)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7)
		RETURNING recorded_at`, event.ID, event.Event, event.OccurredAt.UTC(),
		event.Principal, event.Actor, event.TraceID, attributes)
	if err != nil {
		return businessEventStorageError(operation, err, "failed to append business event")
	}
	event.RecordedAt = recordedAt
	return nil
}

// DeleteRecordedBefore uses one bounded statement outside any transaction carried by ctx.
func (a *Adapter) DeleteRecordedBefore(ctx context.Context, cutoff time.Time, limit int) (deleted int64, err error) {
	const operation = "DeleteRecordedBefore"
	ctx, finish := startStorageSpan(ctx, "storage.delete_recorded_before.business_event", operation)
	defer func() { finish(err) }()
	if a.db == nil {
		return 0, storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	if limit <= 0 {
		return 0, storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "limit must be positive")
	}
	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()
	result, err := a.db.ExecContext(execCtx, `DELETE FROM business_events WHERE id IN (
		SELECT id FROM business_events WHERE recorded_at < $1 ORDER BY recorded_at, id LIMIT $2
	)`, cutoff, limit)
	if err != nil {
		return 0, businessEventStorageError(operation, err, "failed to delete business events")
	}
	deleted, err = result.RowsAffected()
	if err != nil {
		return 0, businessEventStorageError(operation, err, "failed to count deleted business events")
	}
	return deleted, nil
}

func businessEventStorageError(operation string, err error, message string) error {
	if isContextTimeoutOrCanceled(err) || isStatementTimeout(err) {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return storage.NewStorageError(operation, storage.ErrorKindConflict, err, "business event already exists")
	}
	return storage.NewStorageError(operation, storage.ErrorKindConnection, err, message)
}

var _ ports.BusinessEventAppender = (*Adapter)(nil)
var _ ports.BusinessEventRetention = (*Adapter)(nil)
