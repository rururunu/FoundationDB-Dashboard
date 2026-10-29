// Package buildinfo holds the dashboard name and the version stamped at build.
package buildinfo

// Name is the product name shown in settings.
const Name = "foundationdb-dashboard"

// Version is replaced at build with -ldflags. Local builds keep "dev".
var Version = "dev"
