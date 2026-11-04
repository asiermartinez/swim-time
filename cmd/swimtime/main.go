// Command swimtime samples the real-time occupancy of Vitoria-Gasteiz's
// indoor municipal pools and renders charts of the quietest time slots.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	poolsFile   = "pools.json"
	dataDir     = "data"
	chartsDir   = "charts"
	windowWeeks = 8
)

var loc = mustLoadLocation("Europe/Madrid")

// commands maps subcommand names to their implementation. Extra files may
// register more commands from init().
var commands = map[string]func(args []string) error{
	"sample": cmdSample,
	"render": cmdRender,
}

func main() {
	if len(os.Args) < 2 || commands[os.Args[1]] == nil {
		names := make([]string, 0, len(commands))
		for name := range commands {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Fprintf(os.Stderr, "uso: swimtime <%s>\n", strings.Join(names, "|"))
		os.Exit(2)
	}
	if err := commands[os.Args[1]](os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func mustLoadLocation(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}
