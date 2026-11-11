package main

import (
	"fmt"
	"os"

	"github.com/example/deus-rabbitmq-example/internal/config"
	"github.com/example/deus-rabbitmq-example/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: %s [up|down|version]", os.Args[0])
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	switch os.Args[1] {
	case "up":
		if err := migrations.Up(cfg.DatabaseURL); err != nil {
			return err
		}
		fmt.Println("migrations applied")
		return nil
	case "down":
		if err := migrations.Down(cfg.DatabaseURL); err != nil {
			return err
		}
		fmt.Println("migrations rolled back")
		return nil
	case "version":
		version, dirty, err := migrations.Version(cfg.DatabaseURL)
		if err != nil {
			return err
		}
		fmt.Printf("version=%d dirty=%t\n", version, dirty)
		return nil
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}
