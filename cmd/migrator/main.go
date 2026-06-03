// Command migrator is GoStore's CLI wrapper around go-migration's built-in CLI
// (migrate, migrate:rollback, make:migration, make:seeder, db:seed, ...).
// DB-backed commands read connection config from config.yaml.
package main

import (
	"github.com/gopackx/go-migration/pkg/migrator"
)

func main() {
	migrator.Run()
}
