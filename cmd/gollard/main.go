package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yovico-ai/gollard/gollard"
)

const version = "gollard -- 0.6.3.0"

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var err error
	switch cmd {
	case "migrate":
		err = runMigrate(ctx, args)
	case "validate":
		err = runValidate(args)
	case "confirm-checksums":
		err = runConfirmChecksums(ctx, args)
	case "repair-checksum":
		err = runRepairChecksum(ctx, args)
	case "version", "-v", "--version":
		fmt.Println(version)
		return
	case "help", "-h", "--help":
		usage(os.Stdout)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage(w *os.File) {
	fmt.Fprintln(w, "gollard - Database management for pedantic people.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage: gollard COMMAND [OPTIONS]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Available commands:")
	fmt.Fprintln(w, "  migrate              Apply unapplied migrations under a directory.")
	fmt.Fprintln(w, "  validate             Parse a directory and resolve its dependency graph. No database.")
	fmt.Fprintln(w, "  confirm-checksums    Compare applied checksums against the directory.")
	fmt.Fprintln(w, "  repair-checksum      Replace a database checksum with the directory's.")
	fmt.Fprintln(w, "  version              Display application version.")
}

type connOpts struct {
	host     string
	port     int
	user     string
	password string
	database string
}

func registerConnFlags(fs *flag.FlagSet, c *connOpts) {
	fs.StringVar(&c.host, "host", "127.0.0.1", "Server host")
	fs.IntVar(&c.port, "port", 5432, "Server port")
	fs.StringVar(&c.user, "user", "postgres", "Username")
	fs.StringVar(&c.password, "password", "", "Password")
	fs.StringVar(&c.database, "database", "", "Database name")
}

func (c *connOpts) connString() (string, error) {
	if c.database == "" {
		return "", fmt.Errorf("--database is required")
	}
	u := &url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%d", c.host, c.port),
		Path:   "/" + c.database,
	}
	if c.password != "" {
		u.User = url.UserPassword(c.user, c.password)
	} else {
		u.User = url.User(c.user)
	}
	return u.String(), nil
}

func openPool(ctx context.Context, c *connOpts) (*pgxpool.Pool, error) {
	connStr, err := c.connString()
	if err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

// runValidate parses every migration under ROOT and resolves the dependency
// graph. It touches no database, which is the entire point.
//
// Every other command needs a connection, so until now the first thing that
// ever read these files was a deploy. That makes a malformed header a
// deploy-time failure — and a spectacularly broad one, because ImportDirectory
// parses the whole tree before anything is applied: one bad file stops
// migrations that are themselves perfectly fine, in every environment at once.
//
// Checked, in order:
//
//	parse           header syntax and SQL action structure (ImportDirectory)
//	dependency      a `requires` naming a migration that does not exist
//	cycles          MakeMigrationGraph rejects both
//
// Exits non-zero on the first failure so it can be used as a gate.
func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: gollard validate ROOT")
		fs.PrintDefaults()
	}
	if err := parseRootAndFlags(fs, args, "ROOT"); err != nil {
		return err
	}

	planned, tests, err := gollard.ImportDirectory(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := gollard.MakeMigrationGraph(planned); err != nil {
		return err
	}

	fmt.Printf("ok: %d migrations, %d tests, dependency graph resolves\n", len(planned), len(tests))
	return nil
}

func runMigrate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: gollard migrate ROOT [options]")
		fs.PrintDefaults()
	}
	var c connOpts
	registerConnFlags(fs, &c)
	var runTests bool
	fs.BoolVar(&runTests, "test", false, "Run tests after migration.")
	fs.BoolVar(&runTests, "t", false, "Run tests after migration. (shorthand)")
	if err := parseRootAndFlags(fs, args, "ROOT"); err != nil {
		return err
	}
	root := fs.Arg(0)

	pool, err := openPool(ctx, &c)
	if err != nil {
		return err
	}
	defer pool.Close()
	// The mallard bootstrap script ends with `SET search_path TO mallard`
	// (sql/mallard/0000-setup.sql). That is a SESSION setting, so it outlives
	// the script on whichever pooled connection ran it — and a later migration
	// handed that same connection creates its tables in `mallard` instead of
	// `public`. Non-deterministic, because pgxpool decides which connection a
	// query gets, which is what makes it nasty to diagnose.
	//
	// Pin the search_path before applying anything so application DDL always
	// lands where the migration author meant.
	if _, err := pool.Exec(ctx, `SET search_path = public`); err != nil {
		return fmt.Errorf("set migration search_path: %w", err)
	}

	db := gollard.NewDB(pool)
	return gollard.Migrate(ctx, db, root, runTests)
}

func runConfirmChecksums(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("confirm-checksums", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: gollard confirm-checksums ROOT [options]")
		fs.PrintDefaults()
	}
	var c connOpts
	registerConnFlags(fs, &c)
	if err := parseRootAndFlags(fs, args, "ROOT"); err != nil {
		return err
	}
	root := fs.Arg(0)

	pool, err := openPool(ctx, &c)
	if err != nil {
		return err
	}
	defer pool.Close()

	db := gollard.NewDB(pool)
	return gollard.ConfirmChecksums(ctx, db, root)
}

func runRepairChecksum(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("repair-checksum", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: gollard repair-checksum ROOT MIGRATION_NAME [options]")
		fs.PrintDefaults()
	}
	var c connOpts
	registerConnFlags(fs, &c)
	if err := parseRootAndFlags(fs, args, "ROOT", "MIGRATION_NAME"); err != nil {
		return err
	}
	root := fs.Arg(0)
	migrationName := fs.Arg(1)

	pool, err := openPool(ctx, &c)
	if err != nil {
		return err
	}
	defer pool.Close()

	db := gollard.NewDB(pool)
	return gollard.RepairChecksum(ctx, db, root, gollard.MigrationID(migrationName))
}

// parseRootAndFlags lets users mix positional args and flags in any order,
// matching the Haskell CLI's optparse-applicative behavior. Positional args
// are extracted in order; flags (including their values for non-bool flags)
// are forwarded to fs.Parse.
func parseRootAndFlags(fs *flag.FlagSet, args []string, required ...string) error {
	var positional, flagArgs []string
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--":
			// Treat everything after as positional.
			for _, rest := range args[i+1:] {
				positional = append(positional, rest)
			}
			i = len(args)
		case strings.HasPrefix(a, "-") && a != "-":
			flagArgs = append(flagArgs, a)
			i++
			if strings.Contains(a, "=") {
				continue
			}
			name := strings.TrimLeft(a, "-")
			f := fs.Lookup(name)
			if f != nil && !isBoolFlag(f) && i < len(args) {
				flagArgs = append(flagArgs, args[i])
				i++
			}
		default:
			positional = append(positional, a)
			i++
		}
	}
	combined := append(append([]string{}, flagArgs...), positional...)
	if err := fs.Parse(combined); err != nil {
		return err
	}
	if len(positional) < len(required) {
		fs.Usage()
		return fmt.Errorf("missing required argument: %s", required[len(positional)])
	}
	return nil
}

func isBoolFlag(f *flag.Flag) bool {
	if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
		return bf.IsBoolFlag()
	}
	return false
}
