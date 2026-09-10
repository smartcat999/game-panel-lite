package regionexecution

import "context"

func (p *Postgres) Monitoring(ctx context.Context) Monitoring {
	var result Monitoring
	_ = p.db.QueryRowContext(ctx, `SELECT count(*) FROM regional_inbox WHERE handled_at IS NULL`).Scan(&result.InboxLag)
	_ = p.db.QueryRowContext(ctx, `SELECT count(*) FROM regional_outbox WHERE published_at IS NULL`).Scan(&result.OutboxLag)
	_ = p.db.QueryRowContext(ctx, `SELECT count(*) FROM nodes WHERE state = $1`, NodeStale).Scan(&result.StaleNodes)
	_ = p.db.QueryRowContext(ctx, `SELECT count(*) FROM work_assignments WHERE status = $1`, AssignmentFailed).Scan(&result.ReconciliationFailures)
	_ = p.db.QueryRowContext(ctx, `SELECT COALESCE(EXTRACT(EPOCH FROM max(completed_at - created_at)) * 1000, 0)::bigint FROM work_assignments WHERE completed_at IS NOT NULL`).Scan(&result.TaskLatencyMilliseconds)
	return result
}
