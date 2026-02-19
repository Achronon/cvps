package version

import (
	"fmt"
	"runtime"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func String() string {
	commitShort := Commit
	if len(Commit) > 7 {
		commitShort = Commit[:7]
	}
	return fmt.Sprintf("cvps version %s (%s) built %s", Version, commitShort, Date)
}

func Full() string {
	return fmt.Sprintf(
		"cvps version %s\n"+
			"  Commit:     %s\n"+
			"  Built:      %s\n"+
			"  Go version: %s\n"+
			"  OS/Arch:    %s/%s",
		Version, Commit, Date, runtime.Version(), runtime.GOOS, runtime.GOARCH,
	)
}
