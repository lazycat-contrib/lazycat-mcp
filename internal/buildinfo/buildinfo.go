package buildinfo

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

func Snapshot() Info {
	return Info{
		Version:   valueOrDefault(Version, "dev"),
		Commit:    valueOrDefault(Commit, "unknown"),
		BuildTime: valueOrDefault(BuildTime, "unknown"),
	}
}

func valueOrDefault(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
