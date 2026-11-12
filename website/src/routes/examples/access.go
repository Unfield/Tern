package examples

import "github.com/Unfield/Tern/kit"

// Access was added by tern fix access when Tern began closing routes by
// default (ADR 0027): it keeps the route open to everyone, as it was.
// Tighten it where it should be: kit.SignedIn, or kit.Permission("…").
var Access = kit.Public
