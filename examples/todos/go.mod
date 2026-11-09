module todos

go 1.26.0

require github.com/Unfield/Tern v0.0.0

require (
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)

replace github.com/Unfield/Tern => ../..

tool github.com/Unfield/Tern/cmd/tern
