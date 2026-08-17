package buildinfo

import "runtime"

var (
	Version   = "v0.2.4"
	Commit    = "unknown"
	BuildTime = "unknown"
)

type Info struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
	GoVersion string `json:"goVersion"`
}

func Current() Info {
	return Info{
		Name:      "GamePanel Lite",
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
	}
}
