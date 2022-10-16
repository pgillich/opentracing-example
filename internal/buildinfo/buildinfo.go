package buildinfo

// Version is set by the linker.
//nolint:gochecknoglobals // set by the linker
var Version string

// BuildTime is set by the linker.
//nolint:gochecknoglobals // set by the linker
var BuildTime string

// AppName is set by the linker.
//nolint:gochecknoglobals // set by the linker
var AppName string
