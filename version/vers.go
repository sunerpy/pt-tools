package version

var (
	Version   = "unknown" // 默认值，编译时通过 ldflags 覆盖
	BuildTime = "unknown" // 默认值，编译时通过 ldflags 覆盖
	CommitID  = "unknown" // 默认值，编译时通过 ldflags 覆盖
)
