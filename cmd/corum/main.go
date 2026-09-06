package main

import (
	"context"
	"os"

	"github.com/algebananazzzzz/Corum/internal/cli"
)

func main() { os.Exit(cli.RunProcess(context.Background(), os.Args, os.Stdin, os.Stdout, os.Stderr)) }
