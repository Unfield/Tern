package jobs

import (
	"context"
	"strconv"

	"github.com/Unfield/Tern/admin"
	"github.com/Unfield/Tern/kit"
)

// jobRow is a job as the admin screens show it.
type jobRow struct {
	ID          int64   `admin:"key"`
	Kind        string  `admin:"list,search,readonly"`
	State       string  `validate:"oneof=queued|running|done|failed" admin:"list,filter,readonly"`
	Attempts    int     `admin:"list,readonly"`
	MaxAttempts int     `label:"Attempts allowed" admin:"readonly"`
	LastError   *string `label:"Last error" admin:"textarea,readonly,search"`
	Payload     string  `admin:"textarea,readonly"`
	RunAt       int64   `label:"Runs at" admin:"list,readonly,unixms"`
	CreatedAt   int64   `label:"Queued" admin:"readonly,unixms"`
	FinishedAt  *int64  `label:"Finished" admin:"readonly,unixms"`
}

// Admin is a resource for the admin package's screens: the jobs, newest
// first, filtered by state and searched by kind and error, with Retry on
// a failed one and Delete to discard one.
//
//	admin.New(admin.Options{Authorize: onlyAdmins}, q.Admin())
func (q *Queue) Admin(opts ...admin.Option) admin.Res {
	table, err := admin.NewTable[jobRow](q.db, "tern_jobs", "")
	if err != nil {
		panic(err) // the struct is this package's: it can't be wrong
	}
	return admin.Resource("jobs", table, append([]admin.Option{
		admin.Label("Jobs", "Job"),
		admin.NoCreate(),
		admin.Action("retry", "Retry", func(ctx context.Context, key string) error {
			id, _ := strconv.ParseInt(key, 10, 64)
			if err := q.Retry(ctx, id); err != nil {
				return kit.FormError("Only a failed job can be retried.")
			}
			return nil
		}),
	}, opts...)...)
}
