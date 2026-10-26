package main

import (
	"fmt"
	"os"
)

func main() {
	files := map[string]string{
		"world.save": "day=49\n", "serverconfig.txt": "world=existing\n",
		"Mods/kept.mod": "enabled\n", "ModConfigs/kept.json": "{\"enabled\":true}\n",
		".gamepanel-installed": "keep-existing-install\n",
	}
	for name, expected := range files {
		data, err := os.ReadFile("/data/" + name)
		if err != nil || string(data) != expected {
			fmt.Fprintln(os.Stderr, "saved data not restored:", name, err)
			os.Exit(1)
		}
	}
	if err := os.WriteFile("/data/resumed", []byte(files["world.save"]), 0o644); err != nil {
		panic(err)
	}
}
