package accounts

import (
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/signals"
)

// NewDevice is a sign-in from a device the account hadn't been signed in
// on before, for Options.OnNewDevice.
type NewDevice struct {
	User *User
	// Device describes it, such as "Firefox on macOS".
	Device string
	IP     string
	Time   time.Time
}

// noteDevice remembers the device a user signed in on, from
// plugins/signals' device cookie, and tells the user when it's a new one.
// The first device an account is used on isn't news.
func (a *Accounts) noteDevice(c *kit.Ctx, u *User) {
	if !a.opts.NotifyNewDevices && a.opts.OnNewDevice == nil {
		return
	}
	sig := signals.Of(c)
	if sig == nil || sig.Device == "" {
		return
	}
	ctx := c.Request.Context()
	now := time.Now()
	res, err := a.exec(ctx, `INSERT INTO tern_known_devices (user_id, device, first_seen) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, u.ID, sig.Device, now.UnixMilli())
	if err != nil {
		return
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return // known
	}
	var count int
	if err := a.queryRow(ctx, `SELECT COUNT(*) FROM tern_known_devices WHERE user_id = ?`, u.ID).Scan(&count); err != nil || count < 2 {
		return
	}
	d := NewDevice{User: u, Device: describeDevice(c.Request.UserAgent()), IP: c.ClientIP(), Time: now}
	if a.opts.OnNewDevice != nil {
		a.opts.OnNewDevice(c, d)
	}
	if a.opts.NotifyNewDevices && a.opts.Send != nil {
		_ = a.sendWith(c, "new_device", u, "", map[string]any{
			"device": d.Device,
			"ip":     d.IP,
			"time":   d.Time.UTC().Format("2 January 2006, 15:04 UTC"),
		})
	}
}
