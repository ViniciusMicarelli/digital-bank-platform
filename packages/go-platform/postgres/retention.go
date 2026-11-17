package postgres

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var retentionDeleted = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "retention_deleted_rows_total", Help: "Rows removed by retention jobs.",
}, []string{"rule"})

// RetentionRule deletes rows of Table matching Where (which may use $1, the
// cutoff time = now - Keep). Deletes go in small batches so they never hold
// long locks on hot tables.
type RetentionRule struct {
	Name  string
	Table string
	Where string
	Keep  time.Duration
}

// Standard rules for the outbox/inbox tables every service has.
func OutboxRetention(keep time.Duration) RetentionRule {
	return RetentionRule{Name: "outbox", Table: "outbox", Where: "published_at < $1", Keep: keep}
}

func InboxRetention(keep time.Duration) RetentionRule {
	return RetentionRule{Name: "inbox", Table: "inbox", Where: "processed_at < $1", Keep: keep}
}

// ApplyRetention runs each rule until nothing is left to delete.
func ApplyRetention(ctx context.Context, pool *pgxpool.Pool, rules []RetentionRule) (map[string]int64, error) {
	const batch = 1000
	deleted := map[string]int64{}
	for _, r := range rules {
		cutoff := time.Now().Add(-r.Keep)
		for {
			tag, err := pool.Exec(ctx, `DELETE FROM `+r.Table+` WHERE ctid IN (
				SELECT ctid FROM `+r.Table+` WHERE `+r.Where+` LIMIT `+itoa(batch)+`)`, cutoff)
			if err != nil {
				return deleted, err
			}
			n := tag.RowsAffected()
			deleted[r.Name] += n
			retentionDeleted.WithLabelValues(r.Name).Add(float64(n))
			if n < batch || ctx.Err() != nil {
				break
			}
		}
	}
	return deleted, nil
}

// RunRetention applies the rules every interval until ctx ends.
func RunRetention(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, rules []RetentionRule, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := ApplyRetention(ctx, pool, rules); err != nil && ctx.Err() == nil {
			log.WarnContext(ctx, "retention", "error", err)
		} else {
			for rule, c := range n {
				if c > 0 {
					log.InfoContext(ctx, "retention", "rule", rule, "deleted", c)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
