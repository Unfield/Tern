package app

import "strings"

// SafeNext keeps a redirect after signing in on this site: a path, not
// another site's address.
func SafeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/dashboard"
	}
	return next
}
