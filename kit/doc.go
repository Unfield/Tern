// Package kit is the server runtime for Tern applications.
//
// Route files such as +page.server.go import kit to define Load functions,
// form actions and endpoints:
//
//	func Load(ctx *kit.Ctx) (Data, error) {
//		return Data{Title: "Hello"}, nil
//	}
//
// The package is under active development; see
// https://github.com/Unfield/Tern/blob/main/docs/ARCHITECTURE.md
// for the planned API.
package kit
