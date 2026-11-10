package mail

import (
	"context"
	"time"

	"github.com/Unfield/Tern/jobs"
)

// Job registers the "mail.send" job kind, which sends messages with m and
// retries failed deliveries (five attempts by default). Enqueue rendered
// messages on the returned kind.
func Job(q *jobs.Queue, m *Mailer, opts ...jobs.HandleOption) *jobs.Kind[Message] {
	opts = append([]jobs.HandleOption{jobs.MaxAttempts(5), jobs.Timeout(time.Minute)}, opts...)
	return jobs.Handle(q, "mail.send", func(ctx context.Context, msg Message) error {
		return m.Send(ctx, &msg)
	}, opts...)
}
